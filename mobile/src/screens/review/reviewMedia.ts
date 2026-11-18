import type { CardMediaInfo } from '../../api/types';

const CLIP_ORDER: Record<string, number> = { word: 0, sentence: 1 };

/** Select the current audio sidecars in their display order. */
export function selectAudioClips(media: CardMediaInfo[]): CardMediaInfo[] {
  const audio = media.filter((item) => item.kind === 'audio');
  // Labeled clips supersede any unlabeled compatibility row.
  const current = audio.some((item) => item.label)
    ? audio.filter((item) => item.label)
    : audio;
  return [...current].sort(
    (a, b) =>
      (CLIP_ORDER[a.label ?? ''] ?? 2) - (CLIP_ORDER[b.label ?? ''] ?? 2) ||
      a.id - b.id,
  );
}
