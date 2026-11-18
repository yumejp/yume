# Security model

Yume is a self-hosted server for one learner, or a few people who trust each
other. It protects the deck from web pages and from networks it is not bound
to. It does not protect the deck from anyone who can reach its port while it
runs in the default open mode. Report vulnerabilities as described in
[SECURITY.md](../.github/SECURITY.md).

## Where the server listens

- By default the server binds `127.0.0.1:7654`: only programs on the same
  computer can reach it.
- `-bind ADDR` makes it listen on another address. Bind a private address
  such as the machine's Tailscale IP, never a public interface or a LAN you
  do not control. In open mode the server refuses such a bind unless
  `YUME_ALLOW_OPEN_NETWORK=1` confirms the address is private, and it logs a
  note whenever it listens beyond loopback.
- Docker: the container listens on all of its own interfaces, and Compose
  publishes the port on the host's `127.0.0.1` unless `YUME_BIND` names
  another address; Compose sets `YUME_ALLOW_OPEN_NETWORK=1` for that reason.
  Set `YUME_BIND` to a private address only.

For phone access, the [Raspberry Pi recipe](../server/deploy/pi/README.md)
keeps Yume on loopback behind Tailscale Serve, which gives the tailnet an
HTTPS address, with a Tailscale IP bind as the plain-HTTP fallback. It also
shows optional hardening: a one-way tailnet ACL and a firewall that closes
the LAN side.

## Auth modes

`YUME_AUTH_MODE` chooses how a request finds its account:

| | `open` (default) | `required` |
|---|---|---|
| No credentials | acts as the owner | 401 |
| `X-Yume-User` header | acts as the named account | ignored |
| Bearer token from `/auth/login` or `/auth/signup` | that account | that account |
| `/mcp` | open to every caller | the owner's token only |

Open mode has no sign-in: anyone who can reach the port can read and
change every deck, restore backups and use the tutor tools. Use it on
loopback, or on a private network where every device is yours.

Required mode issues opaque session tokens (30 days; only a SHA-256 hash is
stored) and stores bcrypt password hashes. The bundled app opens on a
sign-in screen. The owner account starts without a password: give it one
with `yume-server users owner EMAIL` before switching, or the owner cannot
sign in and `/mcp` is out of reach. New accounts are closed in required mode
unless `YUME_SIGNUP=open`. Run it behind HTTPS if it leaves a private
network, because tokens and passwords travel in plain HTTP otherwise.

`/health`, `/licenses` and `/auth/*` answer without credentials in both modes
and expose no deck data. In required mode the bundled app's own files also
load without a session, so the sign-in screen can appear.

## Defences against web pages

A web page in the learner's browser can try to call a server on
`127.0.0.1` or a private address. Every request passes these checks first
(`server/internal/server/middleware.go`):

1. **Host allowlist.** The Host header must be loopback, the bind address, or
   a name in `YUME_ALLOWED_HOSTS`. This defeats DNS rebinding.
2. **Origin allowlist.** A request carrying an Origin is refused unless the
   Origin is the server itself (the bundled app), a browser extension
   (`chrome-extension://`, `moz-extension://`, `extension://`), or a host
   from the allowlist. `YUME_DEV_CORS=1` also admits `localhost` origins for
   app development; never set it on a shared machine.
3. **JSON only.** Every request other than GET and HEAD must be
   `application/json`, which a cross-origin page cannot send without a
   preflight that check 2 refuses. This blocks cross-site request forgery.
   `/mcp` is exempt because its transport ends sessions with a bodyless
   DELETE; the Host and Origin checks still apply.
4. **Body cap.** Request bodies are limited to 64 MiB.

Native clients, scripts and MCP clients send no Origin and pass check 2.

### Browser extensions

Any installed browser extension's origin passes check 2, so every extension
with access to the page can call the API as the learner. Install only
extensions you trust in a browser that can reach the server.

## The MCP endpoint

`/mcp` serves the tutor tools over Streamable HTTP. The tools act on the
owner's deck: they read the deck and study history, suggest and flag words,
and create and restore deck backups. Treat access to `/mcp` like access to
the deck itself: in open mode, whoever reaches the port can connect a client.

## Data at rest

Everything lives in the data folder: `users.db` (accounts and sessions),
`content.db` (the shared mnemonic library), `users/<id>/cards.db` (each deck)
and card media. The files are not encrypted; protect the folder with the
operating system's permissions and disk encryption, and keep backups
somewhere equally private. [PRIVACY.md](../PRIVACY.md) lists what the server
stores and what it sends elsewhere.

## Logs

Request logs record the route, status and timing, never bodies, query
strings, credentials or user text. See the server guide's
[Request log](../server/README.md#request-log).
