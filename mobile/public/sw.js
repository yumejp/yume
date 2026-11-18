// App-shell service worker: lets the home-screen app open without a
// connection, so the cached queue and the rating outbox are reachable.
//
// The Yume server fills in SHELL when it serves this file: the build's
// version and the list of bundled files (index.html as "/", the hashed
// _expo/static bundle, fonts and icons). Served any other way SHELL stays
// null and the worker does nothing.
//
// Rules: only those bundled files are cached, never API or media responses.
// Pages load network-first, so a reachable server always wins and upgrades
// land; the cached shell is only the fallback when the server is unreachable.
// Each server build gets its own cache, and older caches are dropped once
// the new one is complete.
const SHELL = /* yume:shell */ null;
const PREFIX = 'yume-shell-';
const CACHE = SHELL ? PREFIX + SHELL.version : null;
const FILES = new Set(SHELL ? SHELL.files : []);
// How long a page load waits for the server before opening the cached shell,
// so an unreachable server (Pi off, tailnet down) does not hang the launch.
const NAVIGATE_TIMEOUT_MS = 4000;

self.addEventListener('install', (event) => {
  if (!CACHE) return;
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll([...FILES].map((url) => new Request(url, { cache: 'reload' }))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith(PREFIX) && k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  if (!CACHE) return;
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (req.mode === 'navigate') {
    // Only app pages fall back to the shell; a file such as /media/x.mp3
    // opened in a tab goes to the network untouched.
    if (/\.[^/]*$/.test(url.pathname)) return;
    event.respondWith(navigate(req));
    return;
  }
  if (FILES.has(url.pathname) && url.pathname !== '/') {
    event.respondWith(caches.open(CACHE).then((cache) => cache.match(url.pathname)).then((hit) => hit || fetch(req)));
  }
});

async function navigate(req) {
  const cache = await caches.open(CACHE);
  const shell = await cache.match('/');
  if (!shell) return fetch(req);
  // A proxy in front of a stopped server (tailscale serve, Caddy) answers
  // with a 5xx such as 502: that is unreachable too.
  const network = fetch(req).then((res) => (res.status >= 500 ? shell : res));
  network.catch(() => {}); // a late failure after the timeout is not an error
  let timer;
  const timeout = new Promise((resolve) => {
    timer = setTimeout(() => resolve(shell), NAVIGATE_TIMEOUT_MS);
  });
  try {
    return await Promise.race([network, timeout]);
  } catch {
    return shell;
  } finally {
    clearTimeout(timer);
  }
}
