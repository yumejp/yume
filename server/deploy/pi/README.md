# Yume on a Raspberry Pi (serve your phone over Tailscale)

The zero-cloud-cost deployment: the server runs on an always-on Pi, your
phone opens the app the Pi serves (added to its home screen), and your
desktop and AI tutor reach the same deck. Everything travels over your
private Tailscale network; nothing is exposed to the internet or your LAN.

```
phone (home-screen app) ──┐                       Pi: tailscale serve (HTTPS, :443)
                          ├─ tailnet (WireGuard) ──       └─> yume-server on 127.0.0.1:7654
desktop / AI tutor ───────┘
```

[Tailscale Serve](https://tailscale.com/kb/1312/serve) gives Yume an HTTPS
address, `https://your-pi.your-tailnet.ts.net`, which phones need for
Android's **Install app** and for anything that works while offline. There
is also a [plain-HTTP fallback](#plain-http-fallback).

## Security model

The full model, including auth modes and MCP exposure, is in
[docs/security.md](../../../docs/security.md).

- Yume listens on `127.0.0.1` only. The one way in is Tailscale Serve, which
  answers on the Pi's tailnet name and is invisible to the LAN and the
  internet. Tailscale's free personal plan covers this.
- The Host allowlist admits loopback plus exactly the names you configure
  (`-bind` address auto-allowed, MagicDNS name via `YUME_ALLOWED_HOSTS`).
  DNS-rebinding and browser-CSRF defenses stay fully active.
- Default auth mode is **open**: anyone who can reach the Pi through your
  tailnet, including devices shared into it, acts as the owner account,
  with every deck and the tutor (MCP) tools. Fine for a single-person
  tailnet; don't share the Pi, and never bind a public interface. Open mode
  on a non-loopback bind, such as the plain-HTTP fallback, starts only with
  `YUME_ALLOW_OPEN_NETWORK=1`, your statement that the address is private.
- For separate decks for several people, use `YUME_AUTH_MODE=required` (no
  opt-in needed) and follow
  [Accounts](../../README.md#accounts-separate-decks-on-one-server). Give
  the owner a sign-in first with `yume-server users owner you@example.com`,
  or you lose access to your own deck and the MCP tutor. Behind Tailscale
  Serve every request reaches Yume from `127.0.0.1`, so all devices share
  one sign-in rate limit (10 attempts a minute).
- Enabling HTTPS certificates publishes the machine name and tailnet name
  (for example `your-pi.your-tailnet.ts.net`) in the public Certificate
  Transparency logs. Access is still limited to your tailnet, but pick a
  machine name that reveals nothing you mind being public
  ([Tailscale's note](https://tailscale.com/kb/1153/enabling-https)).

### Optional hardening: one-way, least-privilege tailnet

Default Tailscale is all-devices-to-all-devices. To allow only your devices
→ the Pi (SSH and Yume) and **block the Pi from initiating anything to
your devices**, replace the default allow-all rule in the
[ACL editor](https://login.tailscale.com/admin/acls) with:

```jsonc
{
  "tagOwners": { "tag:server": ["autogroup:admin"] },
  "acls": [
    // devices → Pi on SSH + Yume only (443 for Tailscale Serve; add 7654
    // only for the plain-HTTP fallback). The Pi appears as src in no rule,
    // so Pi → your devices is denied (enforced receiver-side).
    { "action": "accept", "src": ["autogroup:member"], "dst": ["tag:server:22,443"] },
  ],
}
```

Then tag the Pi `tag:server` (admin console → Machines → Edit ACL tags).
Tagged nodes aren't `autogroup:member`, which is what makes the rule one-way.

To also close the Pi's LAN side, so it is reachable *only* through the
tailnet (verify tailnet SSH works first!):

```sh
sudo apt install -y ufw
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow in on tailscale0 to any port 22,443 proto tcp
sudo ufw enable
```

For the plain-HTTP fallback, allow `22,443,7654` instead.

ACLs don't govern plain-LAN traffic, so for an explicit desktop-side
guarantee add a Windows Firewall inbound block of the Pi's LAN IP
(`New-NetFirewallRule -DisplayName "Block yume-pi inbound" -Direction
Inbound -RemoteAddress <pi-lan-ip> -Action Block` in an admin shell), and
give the Pi a DHCP reservation so the IP sticks.

## Setup

1. **Tailscale on the Pi and the phone** (once): https://tailscale.com/download.
   `tailscale status` shows the Pi's MagicDNS name
   (`your-pi.your-tailnet.ts.net`). In the admin console's
   [DNS page](https://login.tailscale.com/admin/dns), make sure MagicDNS is
   on, then select **Enable HTTPS** under HTTPS Certificates (read the
   Certificate Transparency note above first).

2. **Build the server with the phone app inside** (on any machine with
   [Go 1.26.11+](https://go.dev/dl/) and [Node.js 20+](https://nodejs.org/);
   pure Go, no CGO). Without the export step the Pi serves a text notice
   instead of the app.

   ```sh
   cd mobile
   npm ci
   npm run export:server          # bundles the app into server/internal/webapp/dist
   cd ../server
   GOOS=linux GOARCH=arm64 go build -ldflags "-X main.Version=0.1.0-alpha" -o yume-server .   # Pi 3/4/5 (64-bit OS)
   # GOARCH=arm GOARM=7 for a 32-bit OS
   ```

   In Windows PowerShell, set the target first:
   `$env:GOOS="linux"; $env:GOARCH="arm64"; go build -o yume-server .`

3. **Copy it to the Pi and download the dictionaries** (about 21 MB):

   ```sh
   ssh pi@your-pi 'mkdir -p ~/yume'
   scp yume-server pi@your-pi:~/yume/
   ssh pi@your-pi '~/yume/yume-server setup -data ~/yume/data'
   ```

   Moving an existing install? Stop the old server and copy its whole data
   folder to `~/yume/data` instead (see [Backups](#backups)). A single
   `cards.db` from an older build placed in `~/yume/data` moves into the
   owner account on first start; a deck from a pre-release build needs the
   steps in [backup and restore](../../../docs/backup-and-restore.md) first.

4. **Install the service**: edit `deploy/pi/yume-server.service` (user,
   paths, MagicDNS name), then, from `server/`:

   ```sh
   scp deploy/pi/yume-server.service pi@your-pi:~/yume/
   ssh pi@your-pi
   sudo cp ~/yume/yume-server.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now yume-server
   curl http://127.0.0.1:7654/health        # on the Pi: {"ok":true,...}
   ```

5. **Give it an HTTPS address** with Tailscale Serve, on the Pi:

   ```sh
   sudo tailscale serve --bg 7654
   tailscale serve status
   ```

   This proxies `https://your-pi.your-tailnet.ts.net` to
   `http://127.0.0.1:7654` and keeps doing so after reboots (`--bg`).
   - If the command prints a link asking you to enable HTTPS or Serve, open
     it and run the command again.
   - The first request can take a few seconds while the certificate is
     issued.
   - Serve passes the original Host header through, so `YUME_ALLOWED_HOSTS`
     in the unit is all Yume needs.
   - To turn it off: `sudo tailscale serve --https=443 off`.

6. **Open it on the phone**: `https://your-pi.your-tailnet.ts.net`.
   - iPhone: in Safari, tap Share, then **Add to Home Screen**. Do this
     before you sign in or change settings, and use Yume from the icon: the
     home-screen app keeps its own storage, separate from Safari's.
   - Android: in Chrome, open the menu and select **Install app**.
   - AI tutor on another tailnet machine: `https://your-pi.your-tailnet.ts.net/mcp`.
   - Sanity check from the phone's browser: `https://your-pi.your-tailnet.ts.net/health`.

### Plain-HTTP fallback

If you would rather not enable HTTPS certificates, bind Yume to the Pi's
Tailscale IP instead. In the unit:

1. Change `-bind 127.0.0.1` to the address from `tailscale ip -4`.
2. Uncomment `Environment=YUME_ALLOW_OPEN_NETWORK=1` (open mode refuses a
   non-loopback bind without it).
3. Restart, and open `http://your-pi.your-tailnet.ts.net:7654`.

The app works, but on Android **Install app** is not offered over HTTP;
use **Add to Home screen**, which opens Yume in Chrome.

## Updating

1. Build the new version with [Setup](#setup) step 2.
2. Back up (see [Backups](#backups)).
3. Replace the binary, keeping the old one for a rollback:

   ```sh
   scp yume-server pi@your-pi:~/yume/yume-server.new
   ssh pi@your-pi
   sudo systemctl stop yume-server
   mv ~/yume/yume-server ~/yume/yume-server.bak
   mv ~/yume/yume-server.new ~/yume/yume-server
   sudo systemctl start yume-server
   ```

4. Check `https://your-pi.your-tailnet.ts.net/health`, open the app, and
   confirm your library and due count look right.

Never run an older binary against data a newer one has opened; to roll
back, restore the backup as well as `yume-server.bak`.

## Backups

Stop the service and copy the whole data folder to a new, dated place each
time, and keep the server version with it:

```sh
sudo systemctl stop yume-server
mkdir -p ~/yume-backups
cp -a ~/yume/data ~/yume-backups/data-$(date +%F)
sudo systemctl start yume-server
```

The folder holds everything (see
[Data folder](../../README.md#data-folder)), including each deck's `-wal`
file and its `users/<id>/media` (audio and images). Copying it while stopped
keeps the databases and their WAL files consistent. Then copy the backup off
the Pi, from any tailnet machine:

```sh
scp -r pi@your-pi:~/yume-backups/data-2026-09-27 ./
```

See [backup and restore](../../../docs/backup-and-restore.md) for the rules
on restoring.

## Footprint

Measured on a desktop build, not yet on a Pi, with the dictionaries and
example corpus loaded:

- RAM: about 450-600 MB under Go's default garbage collector (live heap
  about 225 MB), or about 330 MB with `GOMEMLIMIT=300MiB` set in the unit.
  It stays roughly flat as users are added.
- CPU: near zero at idle; the dictionary takes a few seconds to load at
  start.

Plan on a Pi with 2 GB of RAM; 1 GB works with `GOMEMLIMIT` set.
