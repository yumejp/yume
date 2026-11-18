import { Platform } from 'react-native';

/** Where the page was loaded from; the subset of `window.location` we read. */
type PageLocation = { protocol: string; hostname: string; port: string };

/**
 * Whether this page may install the app-shell worker (public/sw.js), which
 * lets the home-screen app open offline. Browsers only allow workers on HTTPS
 * or localhost, so a plain http:// LAN or Tailscale-IP address never tries.
 * The Metro dev server (port 8081) is skipped so development never serves a
 * cached bundle.
 */
export function canUseServiceWorker(loc: PageLocation): boolean {
  if (loc.port === '8081') return false;
  if (loc.protocol === 'https:') return true;
  if (loc.protocol !== 'http:') return false;
  const host = loc.hostname.replace(/^\[|\]$/g, '');
  return host === 'localhost' || host.endsWith('.localhost') || host === '::1' || /^127\./.test(host);
}

/** Register the worker once the page has loaded, then ask the browser to
 *  keep this origin's storage (the cached queue and unsent ratings) instead of
 *  evicting it under storage pressure. Web export only; a no-op elsewhere. */
export function registerServiceWorker(): void {
  if (Platform.OS !== 'web' || __DEV__) return;
  if (typeof window === 'undefined' || typeof navigator === 'undefined') return;
  if (!('serviceWorker' in navigator) || !canUseServiceWorker(window.location)) return;
  const register = () => {
    navigator.serviceWorker
      .register('/sw.js')
      .then(() => navigator.serviceWorker.ready)
      .then(() => persistStorage())
      .catch(() => {
        // No worker means no offline start; the app itself works as before.
      });
  };
  if (document.readyState === 'complete') register();
  else window.addEventListener('load', register, { once: true });
}

async function persistStorage(): Promise<void> {
  const storage = navigator.storage;
  if (!storage?.persist) return;
  if (storage.persisted && (await storage.persisted())) return;
  await storage.persist();
}
