import type { YumeClient } from '../api/client';
import type { WatchWord } from '../api/types';

/** Turn a tutor suggestion into a Learning card and withdraw the suggestion.
 *  Checks by lemma first: a suggestion without a dictionary id cannot use the
 *  server's id-based deduplication. An existing card still moves to Learning. */
export async function addSuggestionToDeck(client: YumeClient, w: WatchWord): Promise<void> {
  const status = await client.cardStatus([], [w.lemma]).catch(() => null);
  const existingCard = status?.lemma_statuses?.[0]?.card_id ?? null;
  if (existingCard == null) {
    const results = await client.lookup([w.lemma]).catch(() => []);
    const entry = results.flatMap((r) => r.entries)[0];
    await client.saveCard({
      lemma: w.lemma,
      jmdict_id: entry?.id,
      kanji: entry?.kanji,
      readings: entry?.readings,
      glosses: entry?.senses?.map((s) => s.glosses ?? []),
    });
  }
  await client.setWordStatus(w.lemma, 1);
  await client.removeWatchWord(w.lemma);
}
