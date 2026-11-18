import { useEffect, useState } from 'react';
import { useServer } from '../server/ServerContext';
import { invalidate, useInvalidationVersion, useServerQuery } from './queryStore';

// The last state this app set, shared by every screen until stats confirm it;
// without it a screen can show the old state and a tap there undoes the change.
let pending: number | null | undefined;
let inFlight = false;

/** Vacation mode: when it started (null when off) and a switch for it. */
export function useVacation() {
  const server = useServer();
  const live = server.live;
  const query = useServerQuery('stats', live ? () => server.client!.deckStats() : null, [live]);
  useInvalidationVersion('stats'); // re-render every screen when the mode changes
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fetched = live ? query.data?.vacation_since ?? null : null;
  useEffect(() => {
    if (!inFlight && pending !== undefined && fetched === pending) pending = undefined;
  }, [fetched]);
  const since = pending !== undefined ? pending : fetched;

  const setOn = async (on: boolean) => {
    if (inFlight) return;
    inFlight = true;
    const before = since;
    setBusy(true);
    setError(null);
    pending = on ? Math.floor(Date.now() / 1000) : null;
    invalidate('stats');
    try {
      const res = await server.client!.setVacation(on);
      pending = res.vacation_since || null;
    } catch (e) {
      pending = before;
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      inFlight = false;
      setBusy(false);
      invalidate('stats');
    }
  };
  // Until stats arrive the state is unknown; a switch shown as off would invite a wrong tap.
  const loaded = live && query.data != null;
  return { live, loaded, since, busy, settling: busy, error, setOn };
}

export function sinceLabel(since: number): string {
  return new Date(since * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
