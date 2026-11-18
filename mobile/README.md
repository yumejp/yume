# Yume Mobile

The Expo app included with Yume, for phone and web browser. Your deck lives
on your own Yume server; the [user guide](../docs/user-guide.md) describes
the four tabs (**Review**, **Library**, **Dictionary**, **Settings**).

## Connect to a server

When the server serves the app (see the root README), the app talks to the
server it was loaded from and nothing needs configuring. Running from source
instead, open Settings → Connection and enter the address of your running
server, such as `http://100.x.y.z:7654` for a machine on your Tailscale
network. The app saves the address on the device and syncs study settings
through the server.

When the server runs in required auth mode, the app opens on a sign-in screen
and Settings shows the signed-in account; in the default open mode there is
no sign-in.

When the server is unavailable, Review uses a cached due queue and replays
ratings after reconnecting, or offers a clearly labeled sample deck if no
real deck has been cached.

## Stack

- **[Expo](https://docs.expo.dev/) SDK 56** + React Native 0.85 + React 19
- **TypeScript**
- Design tokens in `src/theme.ts`: warm paper surfaces, plum text, coral
  actions, and a gold crescent moon.

## Repository layout

| Piece | Location | Role |
| --- | --- | --- |
| Local server | `../server` (Go) | Tokenizer (kagome), JMdict, FSRS, SQLite |
| **Mobile app** | **this folder** | Study client: dictionary, mining, reviews |

Every client of the same server shares one deck, and all clients leave
scheduling to the Go server.

## Develop

```bash
npm install
npm start        # Expo dev server (scan QR with Expo Go)
npm run android  # build/run on Android device or emulator
npm run ios      # requires macOS, or use Expo Go
npm run web      # run in the browser
npm test         # Jest suite
npx tsc --noEmit # TypeScript check
```

The app must remain compatible with Expo Go. Avoid dependencies that require a
custom native build.
