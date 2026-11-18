import { useEffect } from 'react';
import { Platform } from 'react-native';
import { RATINGS, type Phase } from '../useReviewSession';

/** Web shortcuts: Space/Enter reveals, 1–4 rates, Z undoes. */
export function useReviewKeyboard({
  active,
  phase,
  submitting,
  canUndo,
  blocked,
  rate,
  undo,
  reveal,
}: {
  active: boolean;
  phase: Phase;
  submitting: boolean;
  canUndo: boolean;
  /** An open dialog/menu: rating under it would advance the queue beneath it. */
  blocked: boolean;
  rate: (r: (typeof RATINGS)[number]) => Promise<void>;
  undo: () => Promise<void>;
  reveal: () => void;
}): void {
  useEffect(() => {
    // Bound only while this tab is visible — a hidden, still-mounted Review
    // would otherwise steal keypresses on other tabs.
    if (!active || Platform.OS !== 'web' || phase === 'idle' || phase === 'loading' || phase === 'done') return;
    const W = globalThis as any;
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA') return;
      if (blocked) return;
      if ((e.key === 'z' || e.key === 'Z') && canUndo && !submitting) {
        e.preventDefault();
        void undo();
        return;
      }
      if (phase === 'front' && (e.key === ' ' || e.key === 'Enter')) {
        e.preventDefault();
        reveal();
        return;
      }
      if (phase === 'back' && !submitting) {
        const idx = ['1', '2', '3', '4'].indexOf(e.key);
        if (idx >= 0) {
          e.preventDefault();
          void rate(RATINGS[idx]);
        }
      }
    };
    W.addEventListener('keydown', onKey);
    return () => W.removeEventListener('keydown', onKey);
  }, [active, phase, submitting, canUndo, blocked, rate, undo, reveal]);
}

