import { useEffect } from 'react';
import { invalidate } from '../data/queryStore';
import { loadOutbox, replayOutbox } from '../data/offlineReview';
import { useServer } from './ServerContext';

/**
 * Replays the offline rating outbox whenever the server comes back into
 * reach (status flips to 'online' — app foregrounding re-probes handle the
 * "walked back into wifi" case). Mounted once inside ServerProvider, like
 * SettingsSync. Session-start has its own drain (a live queue must never be
 * fetched over unsent ratings); replayOutbox single-flights the overlap.
 */
export function OfflineSync() {
  const server = useServer();
  const { status, client, identity } = server;

  useEffect(() => {
    if (status !== 'online' || !client) return;
    let cancelled = false;
    // Loops because an offline session keeps rating while the drain runs;
    // entries appended after the snapshot would otherwise wait for the next
    // status flip. Stops when nothing moved (network failure mid-run).
    const drain = async () => {
      while (!cancelled) {
        const outbox = await loadOutbox(identity);
        if (outbox.length === 0) return;
        const res = await replayOutbox(client, identity);
        // Synced reviews move due counts and history — refresh everything
        // riding 'stats' (tab badge, idle screen, progress). Conflicts
        // (skipped) didn't change the server but did drain the outbox, and
        // the pending-sync count rides the same refresh.
        if (res.sent === 0 && res.skipped === 0) return;
        invalidate('stats');
      }
    };
    drain().catch(() => { /* next online flip retries */ });
    return () => { cancelled = true; };
  }, [status, client, identity]);

  return null;
}
