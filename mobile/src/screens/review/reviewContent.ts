import type { YumeClient } from '../../api/client';
import type {
  CardMediaInfo,
  CardNote,
  DueCard,
  ExampleSentence,
  GetCardMediaResponse,
  MediaBlob,
} from '../../api/types';
import { prefetchSentence } from '../../components/JapaneseSentence';
import { dueCardToVM } from '../../data/live';
import { cachePut } from '../../lib/cache';
import { japaneseDefinition } from '../../lib/definitions';
import { stripMarkup } from '../../lib/text';
import { selectAudioClips } from './reviewMedia';

const CONTENT_CACHE_MAX = 100;
const AUDIO_BYTES_CACHE_MAX = 24;

// Examples are shared corpus data. Notes and media are account-owned, and
// their numeric ids may overlap across accounts.
const examplesCache = new Map<string, ExampleSentence[]>();
// lemma -> Japanese definition (null when the dictionary has none).
const definitionCache = new Map<string, string | null>();
const notesCache = new Map<number, CardNote[]>();
const clipListCache = new Map<number, CardMediaInfo[]>();
const clipBytesCache = new Map<number, MediaBlob>();
const primaryMediaCache = new Map<number, GetCardMediaResponse>();
// Fetches in flight, so a prefetch and a tap (or the image and the audio)
// share one request instead of racing two.
const clipBytesInflight = new Map<number, Promise<MediaBlob | null>>();
const primaryMediaInflight = new Map<number, Promise<GetCardMediaResponse | null>>();

function cleanExamples(examples: ExampleSentence[]): ExampleSentence[] {
  return examples.map((example) => ({
    ...example,
    text: stripMarkup(example.text),
    translation: stripMarkup(example.translation),
  }));
}

function prefetchNoteSentences(client: YumeClient, notes: CardNote[]): void {
  for (const note of notes) {
    if (note.kind === 'example') prefetchSentence(client, note.note);
  }
}

export function clearAccountContent(): void {
  notesCache.clear();
  clipListCache.clear();
  clipBytesCache.clear();
  primaryMediaCache.clear();
  clipBytesInflight.clear();
  primaryMediaInflight.clear();
}

export function clearSessionContent(): void {
  notesCache.clear();
  clipListCache.clear();
  primaryMediaCache.clear();
  primaryMediaInflight.clear();
}

export async function loadExamples(
  client: YumeClient,
  lemma: string,
  reading?: string,
): Promise<ExampleSentence[]> {
  const cached = examplesCache.get(lemma);
  if (cached) return cached;
  const examples = cleanExamples(await client.examples([lemma], 3, 'ja', reading));
  cachePut(examplesCache, lemma, examples, CONTENT_CACHE_MAX);
  examples.forEach((example) => prefetchSentence(client, example.text));
  return examples;
}

/** Cached notes for a card, if it was prefetched — lets the card render its
 *  mnemonic in the same frame instead of one tick later. */
export function peekNotes(cardId: number): CardNote[] | undefined {
  return notesCache.get(cardId);
}

export async function loadNotes(
  client: YumeClient,
  cardId: number,
  isCurrentClient: () => boolean,
): Promise<CardNote[]> {
  const cached = notesCache.get(cardId);
  if (cached) return cached;
  const notes = await client.listCardNotes(cardId);
  if (!isCurrentClient()) return [];
  cachePut(notesCache, cardId, notes, CONTENT_CACHE_MAX);
  prefetchNoteSentences(client, notes);
  return notes;
}

/** A note the learner just wrote joins the card's cached notes (newest
 *  first, like the server), so the card keeps it without a re-fetch. */
export function addCachedNote(cardId: number, note: CardNote): void {
  const cached = notesCache.get(cardId);
  if (cached) cachePut(notesCache, cardId, [note, ...cached], CONTENT_CACHE_MAX);
}

export async function loadAudioClips(
  client: YumeClient,
  cardId: number,
  isCurrentClient: () => boolean,
): Promise<CardMediaInfo[]> {
  const cached = clipListCache.get(cardId);
  if (cached) return cached;
  const clips = selectAudioClips(await client.listCardMedia(cardId));
  if (!isCurrentClient()) return [];
  cachePut(clipListCache, cardId, clips, CONTENT_CACHE_MAX);
  return clips;
}

/** Clip bytes already fetched, so a tap can play them without an await. */
export function peekAudioBytes(mediaId: number): MediaBlob | undefined {
  return clipBytesCache.get(mediaId);
}

export async function loadAudioBytes(
  client: YumeClient,
  mediaId: number,
  isCurrentClient: () => boolean,
): Promise<MediaBlob | null> {
  const cached = clipBytesCache.get(mediaId);
  if (cached) return cached;
  const pending = clipBytesInflight.get(mediaId);
  if (pending) return pending;
  const load = (async () => {
    try {
      const audio = await client.getCardMediaFile(mediaId);
      if (!audio?.data || !isCurrentClient()) return null;
      cachePut(clipBytesCache, mediaId, audio, AUDIO_BYTES_CACHE_MAX);
      return audio;
    } finally {
      clipBytesInflight.delete(mediaId);
    }
  })();
  clipBytesInflight.set(mediaId, load);
  return load;
}

/** A card's primary media (clip and image) if already fetched. */
export function peekPrimaryMedia(cardId: number): GetCardMediaResponse | undefined {
  return primaryMediaCache.get(cardId);
}

export async function loadPrimaryMedia(
  client: YumeClient,
  cardId: number,
  isCurrentClient: () => boolean,
): Promise<GetCardMediaResponse | null> {
  const cached = primaryMediaCache.get(cardId);
  if (cached) return cached;
  const pending = primaryMediaInflight.get(cardId);
  if (pending) return pending;
  const load = (async () => {
    try {
      const media = await client.getCardMedia(cardId);
      if (!isCurrentClient()) return null;
      cachePut(primaryMediaCache, cardId, media, AUDIO_BYTES_CACHE_MAX);
      return media;
    } finally {
      primaryMediaInflight.delete(cardId);
    }
  })();
  primaryMediaInflight.set(cardId, load);
  return load;
}

/** Fetch a card's audio ahead of the tap that plays it: the clip list, each
 *  clip's bytes and the primary clip. A tap then plays from cache with no
 *  await, which iPhone Safari requires. */
export function prefetchCardAudio(
  client: YumeClient,
  cardId: number,
  isCurrentClient: () => boolean,
): Promise<CardMediaInfo[]> {
  void loadPrimaryMedia(client, cardId, isCurrentClient).catch(() => {});
  return loadAudioClips(client, cardId, isCurrentClient).then((clips) => {
    for (const clip of clips) void loadAudioBytes(client, clip.id, isCurrentClient).catch(() => {});
    return clips;
  });
}

export async function loadCardImage(
  client: YumeClient,
  cardId: number,
  isCurrentClient: () => boolean,
): Promise<MediaBlob | null> {
  return (await loadPrimaryMedia(client, cardId, isCurrentClient))?.image ?? null;
}

/** The Japanese definition for a lemma, from the server's imported
 *  dictionary; cached per lemma so a prefetched card never reflows. */
export async function loadJapaneseDefinition(client: YumeClient, lemma: string): Promise<string | null> {
  const cached = definitionCache.get(lemma);
  if (cached !== undefined) return cached;
  const results = await client.lookup([lemma]);
  const def = japaneseDefinition(results.flatMap((r) => r.entries));
  cachePut(definitionCache, lemma, def, CONTENT_CACHE_MAX);
  return def;
}

/** Warm layout-sensitive content before the next card becomes visible. */
export function prefetchReviewCard(
  client: YumeClient,
  dueCard: DueCard,
  showExamples: boolean,
  isCurrentClient: () => boolean,
  japaneseDefinitions = false,
): void {
  const card = dueCardToVM(dueCard);
  if (card.sentence) prefetchSentence(client, card.sentence);
  if (showExamples) void loadExamples(client, card.lemma, card.reading).catch(() => {});
  if (japaneseDefinitions) void loadJapaneseDefinition(client, card.lemma).catch(() => {});
  if (card.id < 0) return;
  if (card.audioPath) {
    void prefetchCardAudio(client, card.id, isCurrentClient).catch(() => {});
  }
  if (card.imagePath) {
    void loadCardImage(client, card.id, isCurrentClient).catch(() => {});
  }
  void loadNotes(client, card.id, isCurrentClient).catch(() => {});
}
