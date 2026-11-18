import AsyncStorage from '@react-native-async-storage/async-storage';
import { useCallback, useEffect, useState } from 'react';

const STORAGE_KEY = 'yume.seenNoteIds';
const MAX_IDS = 500;

/** Persist which tutor notes have already auto-expanded for the learner. */
export function useSeenNotes(): {
  seenNotes: Set<number>;
  markNoteSeen: (id: number) => void;
} {
  const [seenNotes, setSeenNotes] = useState<Set<number>>(new Set());

  useEffect(() => {
    AsyncStorage.getItem(STORAGE_KEY)
      .then((raw) => {
        if (!raw) return;
        try {
          setSeenNotes(new Set(JSON.parse(raw)));
        } catch {
          // A corrupt preference should not block review.
        }
      })
      .catch(() => {});
  }, []);

  const markNoteSeen = useCallback((id: number) => {
    setSeenNotes((current) => {
      if (current.has(id)) return current;
      let next = new Set(current);
      next.add(id);
      if (next.size > MAX_IDS) next = new Set([...next].slice(-MAX_IDS));
      void AsyncStorage.setItem(STORAGE_KEY, JSON.stringify([...next])).catch(() => {});
      return next;
    });
  }, []);

  return { seenNotes, markNoteSeen };
}
