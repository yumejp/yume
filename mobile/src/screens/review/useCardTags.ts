import { useEffect, useState } from 'react';
import type { YumeClient } from '../../api/client';
import type { DueCard } from '../../api/types';

/** System tags on the current card ('flagged', 'needs-explanation', …): the
 *  AI tutor picks these up later; review never reads them. */
export function useCardTags({
  live,
  client,
  currentDue,
}: {
  live: boolean;
  client: YumeClient | null;
  currentDue: DueCard | null;
}): {
  isCardTagged: (tag: string, id: number) => boolean;
  toggleCardTag: (tag: string) => Promise<void>;
  savingTag: string | null;
  error: string | null;
  clearError: () => void;
} {
  // Optimistic per-tag overrides (cardId → tagged?), layered over the
  // server-seeded `system_tags`; absent = whatever the server sent.
  const [tagOverrides, setTagOverrides] = useState<Record<string, Record<number, boolean>>>({});
  const [savingTag, setSavingTag] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Card ids overlap across accounts, so overrides die with the client identity.
  useEffect(() => {
    setTagOverrides({});
  }, [client]);

  const isCardTagged = (tag: string, id: number) =>
    tagOverrides[tag]?.[id] ?? currentDue?.system_tags?.includes(tag) ?? false;

  // Optimistic toggle, reverted if the write fails.
  const toggleCardTag = async (tag: string) => {
    if (!live || !client || !currentDue || currentDue.id < 0) return;
    const id = currentDue.id;
    const willAdd = !isCardTagged(tag, id);
    const setTo = (tagged: boolean) =>
      setTagOverrides((prev) => ({ ...prev, [tag]: { ...prev[tag], [id]: tagged } }));
    setTo(willAdd);
    setSavingTag(tag);
    setError(null);
    try {
      await client.tagCard(id, tag, !willAdd);
    } catch {
      setTo(!willAdd);
      setError('Couldn’t save that request. Check your connection and try again.');
    } finally {
      setSavingTag(null);
    }
  };

  return { isCardTagged, toggleCardTag, savingTag, error, clearError: () => setError(null) };
}

