import { useEffect, useState } from 'react';
import { useServer } from '../server/ServerContext';
import { invalidate, useInvalidationVersion, useServerQuery } from './queryStore';

// The last limit this app set, shown until stats confirm it (see vacation.ts).
let pending: number | undefined;

/** The daily review limit (0 = none) and a setter for it. */
export function useReviewLimit() {
  const server = useServer();
  const live = server.live;
  const query = useServerQuery('stats', live ? () => server.client!.deckStats() : null, [live]);
  useInvalidationVersion('stats');
  const [error, setError] = useState<string | null>(null);
  const fetched = live ? query.data?.review_limit : undefined;
  useEffect(() => {
    if (pending !== undefined && fetched === pending) pending = undefined;
  }, [fetched]);
  const limit = pending ?? fetched;

  const setLimit = async (n: number) => {
    const before = limit;
    pending = n;
    setError(null);
    invalidate('stats');
    try {
      await server.client!.setReviewLimit(n);
    } catch (e) {
      pending = before;
      setError(e instanceof Error ? e.message : String(e));
    }
    invalidate('stats');
  };
  return { limit, error, setLimit };
}
