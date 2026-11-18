/**
 * Put a value into a size-capped Map cache, evicting the oldest insertion
 * once `max` entries is exceeded (Map preserves insertion order). The one
 * capped-cache helper for the app's little prefetch caches — examples, notes,
 * audio clip lists/bytes — which were each growing their own copy of this.
 */
export function cachePut<K, V>(m: Map<K, V>, k: K, v: V, max: number): void {
  m.set(k, v);
  if (m.size > max) {
    const oldest = m.keys().next().value;
    if (oldest !== undefined) m.delete(oldest);
  }
}
