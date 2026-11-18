import { useEffect, useRef } from 'react';
import type { YumeClient } from '../api/client';
import { useServer } from '../server/ServerContext';
import { useSettings, type Settings } from './SettingsContext';

/**
 * Bridges SettingsContext and ServerContext (mounted inside <ServerProvider/>
 * because SettingsProvider wraps it): keeps the per-account server copy of the
 * settings authoritative across devices.
 *
 * - On coming online (once per client identity — i.e. per account/URL), pull
 *   /get_settings and overlay it on the local copy. Server wins: the local
 *   copy is a cache the phone browser may have wiped back to defaults.
 * - After that pull, push local changes back via /save_settings, debounced.
 *   Never push before the pull has landed — a fresh install pushing DEFAULTS
 *   would wipe the account's real settings for every other device.
 *
 * If the sync endpoints are unavailable, device persistence remains active.
 */
export function SettingsSync() {
  const { settings, loaded, hydrate } = useSettings();
  const server = useServer();

  // The client object's identity IS the account identity: ServerContext mints
  // a new YumeClient on URL/token/devUser change. Pull once per identity.
  const pulledForRef = useRef<YumeClient | null>(null);
  const lastPushedRef = useRef<string | null>(null);

  useEffect(() => {
    const client = server.client;
    if (!loaded || server.status !== 'online' || !client) return;
    if (pulledForRef.current === client) return;
    let cancelled = false;
    client
      .getSettings()
      .then((resp) => {
        if (cancelled) return;
        pulledForRef.current = client;
        if (resp.value) {
          hydrate(resp.value as Partial<Settings>);
          lastPushedRef.current = JSON.stringify({ ...resp.value });
        } else {
          // Nothing server-side yet (first device): seed it with the local
          // copy so the account has a baseline to hand to the next device.
          lastPushedRef.current = null;
        }
      })
      .catch(() => {
        /* endpoint missing / offline blip — retry on next status change */
      });
    return () => {
      cancelled = true;
    };
  }, [loaded, server.status, server.client, hydrate]);

  useEffect(() => {
    const client = server.client;
    if (server.status !== 'online' || !client) return;
    if (pulledForRef.current !== client) return; // never push before pulling
    const blob = JSON.stringify(settings);
    if (blob === lastPushedRef.current) return;
    const timer = setTimeout(() => {
      client
        .saveSettings(settings as unknown as Record<string, unknown>)
        .then(() => {
          lastPushedRef.current = blob;
        })
        .catch(() => {
          /* Device persistence remains active; retry on the next change. */
        });
    }, 800);
    return () => clearTimeout(timer);
  }, [settings, server.status, server.client]);

  return null;
}
