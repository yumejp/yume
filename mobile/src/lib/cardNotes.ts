/**
 * Card notes. Every note carries a `kind` set by the server
 * (tutor | example | triage); an example's translation is its own field
 * and the text is rendered as-is. The card's own sentence,
 * translation, explanation and mnemonic are card fields (`ReviewCardVM`).
 */

import type { CardNoteKind } from '../api/types';

type NoteLike = { kind: CardNoteKind };

const RANK: Record<CardNoteKind, number> = { example: 0, mnemonic: 1, tutor: 2, triage: 3 };

/** Vertical order of a card's notes: example sentences first, then tutor
 *  notes. Stable within each rank, so same-kind notes keep their server
 *  order. */
export function noteRank(n: NoteLike): number {
  return RANK[n.kind] ?? RANK.tutor;
}

export function orderNotes<T extends NoteLike>(notes: T[]): T[] {
  return [...notes].sort((a, b) => noteRank(a) - noteRank(b));
}
