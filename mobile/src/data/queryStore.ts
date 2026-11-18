import { errorText } from '../lib/errors';
import { useCallback, useEffect, useRef, useState } from 'react';

/** Cross-tab invalidation for mounted screens and time-sensitive queries. */

export type QueryKey = 'stats' | 'vocab' | 'suggested';

const versions: Record<QueryKey, number> = { stats: 0, vocab: 0, suggested: 0 };
const listeners = new Set<() => void>();
const bootAt = Date.now();
const lastBumpAt: Record<QueryKey, number> = { stats: bootAt, vocab: bootAt, suggested: bootAt };

/** Notify subscribers that the given query keys changed. */
export function invalidate(...keys: QueryKey[]): void {
  for (const k of keys) {
    versions[k]++;
    lastBumpAt[k] = Date.now();
  }
  listeners.forEach((fn) => fn());
}

/** Stats become stale as cards reach their due time. */
export const STATS_STALE_MS = 60_000;

/** Vocabulary content changes through local or remote writes. */
export const CONTENT_STALE_MS = 5 * 60_000;

/** Invalidate keys that have not been refreshed within maxAgeMs. */
export function invalidateIfStale(maxAgeMs: number, ...keys: QueryKey[]): void {
  const stale = keys.filter((k) => Date.now() - lastBumpAt[k] > maxAgeMs);
  if (stale.length > 0) invalidate(...stale);
}

/** Re-render subscriber whenever `key` is invalidated. */
export function useInvalidationVersion(key: QueryKey): number {
  const [v, setV] = useState(versions[key]);
  useEffect(() => {
    const fn = () => setV(versions[key]);
    listeners.add(fn);
    fn(); // catch a bump between render and subscribe
    return () => {
      listeners.delete(fn);
    };
  }, [key]);
  return v;
}

interface QueryResult<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  /** Manual refetch (pull-to-refresh). */
  reload: () => void;
}

/**
 * Fetch server data tied to an invalidation key. Refetches on invalidation,
 * `deps` change, or reload(). fetcher=null resets data and fires nothing.
 * Stale responses are dropped. The fetcher is intentionally NOT a dependency;
 * gate refetches through `deps`.
 */
export function useServerQuery<T>(
  key: QueryKey,
  fetcher: (() => Promise<T>) | null,
  deps: readonly unknown[],
): QueryResult<T> {
  const version = useInvalidationVersion(key);
  const [nonce, setNonce] = useState(0);
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const seq = useRef(0);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  useEffect(() => {
    const f = fetcherRef.current;
    const id = ++seq.current;
    if (!f) {
      setData(null);
      setError(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    f()
      .then((d) => {
        if (seq.current !== id) return;
        setData(d);
        setError(null);
      })
      .catch((e) => {
        if (seq.current !== id) return;
        setError(errorText(e));
      })
      .finally(() => {
        if (seq.current === id) setLoading(false);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, nonce, !fetcher, ...deps]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { data, loading, error, reload };
}
