/**
 * Content a learner writes on a card without a tutor. Each edit goes where
 * docs/card-content.md puts it: the mnemonic story, translation and
 * explanation are card fields; an extra sentence is an `example` note.
 */

import type { YumeClient } from '../api/client';
import type { CardNote, DueCard } from '../api/types';

export type CardEdit =
  | { kind: 'mnemonic'; story: string }
  | { kind: 'example'; sentence: string; translation: string }
  /** translation is left out when the card has no sentence to translate. */
  | { kind: 'meaning'; translation?: string; explanation: string };

export interface CardEditResult {
  /** New field values for the card in the review queue. */
  patch?: Partial<DueCard>;
  /** The note the server stored, for the card's note list. */
  note?: CardNote;
}

/** Write one edit through the existing endpoints and describe what changed. */
export async function writeCardEdit(client: YumeClient, due: DueCard, edit: CardEdit): Promise<CardEditResult> {
  switch (edit.kind) {
    case 'mnemonic': {
      const story = edit.story.trim();
      // The learner writes the story; the breakdown tiles stay as they are.
      const parts = due.mnemonic?.parts ?? [];
      await client.setMnemonic(due.id, parts.map(({ kanji, meaning }) => ({ kanji, meaning })), story);
      return { patch: { mnemonic: { parts, story }, mnemonic_shared: false } };
    }
    case 'example': {
      // One line: sent without a translation, a second line would become one.
      const sentence = edit.sentence.replace(/\s*\n\s*/g, '').trim();
      const translation = edit.translation.trim();
      const { id, created_at } = await client.addCardNote(due.id, 'example', sentence, translation || undefined);
      return {
        note: { id, card_id: due.id, kind: 'example', note: sentence, translation: translation || undefined, created_at },
      };
    }
    case 'meaning': {
      const explanation = edit.explanation.trim();
      const translation = edit.translation?.trim();
      await client.updateCard(due.id, translation === undefined ? { explanation } : { translation, explanation });
      const patch: Partial<DueCard> = { explanation: explanation || null };
      if (translation !== undefined) patch.translation = translation || null;
      return { patch };
    }
  }
}
