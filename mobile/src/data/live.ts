import type { DashboardStats, DueCard, DeckStats as WireDeckStats, Mnemonic, VocabRow, WordStatusValue } from '../api/types';
import { stripMarkup } from '../lib/text';
import type { Card, DayActivity, DeckStats, WordStatus } from './types';

const KANJI_RE = /[一-鿿㐀-䶿]/;

/** Numeric word status (wire) → the app's string union. */
export function statusName(s: WordStatusValue): WordStatus {
  return s === 1 ? 'learning' : s === 2 ? 'known' : s === 3 ? 'ignored' : 'unknown';
}

/** Compact time until a rating's next review ("10m", "3d", "2mo"). */
/** Label for a future time relative to `nowSec` (default: now). Pass the
 *  moment a due list was fetched so a 24h preview stays "24h" for the whole
 *  session; the server re-applies the floor from the actual rating time. */
export function formatWhen(unixSec?: number, nowSec: number = Date.now() / 1000): string {
  if (!unixSec) return '';
  const s = Math.max(0, unixSec - nowSec);
  if (s < 45) return '<1m';
  if (s < 3600) return `${Math.round(s / 60)}m`;
  if (s < 86400) return `${Math.round(s / 3600)}h`;
  if (s < 86400 * 30) return `${Math.round(s / 86400)}d`;
  if (s < 86400 * 365) return `${Math.round(s / (86400 * 30))}mo`;
  return `${(s / (86400 * 365)).toFixed(1)}y`;
}

/** Due review count, excluding new cards that have not been introduced. */
export function gatedDueCount(s: WireDeckStats): number {
  // During catch-up a session promises today's share, not the whole backlog.
  if (s.catch_up) return s.catch_up.remaining_today;
  return s.due_reviews ?? s.due;
}

/** First sense's glosses, joined — the one-line summary used in lists. */
export function glossLine(glosses?: string[][]): string {
  if (!glosses || glosses.length === 0) return '';
  return glosses
    .map((sense) => sense.join(', '))
    .filter(Boolean)
    .slice(0, 3)
    .join(' · ');
}

export function hostOf(url?: string | null): string | undefined {
  if (!url) return undefined;
  const m = url.match(/^[a-z]+:\/\/([^/:]+)/i);
  return m ? m[1] : url;
}

/** Server vocab row → the screen's Card view-model. */
export function vocabRowToCard(row: VocabRow, i: number): Card {
  return {
    id: row.card_id ?? -(i + 1), // statuses-without-cards need a stable key
    // The lemma is the headword; dictionary spellings may be uncommon forms.
    lemma: row.lemma,
    reading: row.readings?.[0],
    glosses: row.glosses?.[0] ?? [],
    sentence: row.sentence ? stripMarkup(row.sentence) || undefined : undefined,
    source: hostOf(row.source_url),
    tags: row.tags ?? [],
    status: statusName(row.status),
    dueInDays: 0, // not on this wire shape; unused by the vocab list
    band: row.band,
  };
}

export function localISO(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

/** Server dashboard payload → the Progress screen's view shape.
 *  The sparse activity heatmap is densified into the last 12 weeks. */
export function statsToDeckStats(s: DashboardStats): DeckStats {
  const byDate = new Map((s.heatmap ?? []).map((d) => [d.date, d.count]));
  const history: DayActivity[] = [];
  const today = new Date();
  for (let i = 12 * 7 - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setDate(today.getDate() - i);
    const key = localISO(d);
    history.push({ date: key, reviews: byDate.get(key) ?? 0 });
  }

  const forecastByDate = new Map((s.due_forecast ?? []).map((d) => [d.date, d.count]));
  const dueForecast = [0, 1, 2, 3, 4, 5, 6].map((offset) => {
    const d = new Date(today);
    d.setDate(today.getDate() + offset);
    return forecastByDate.get(localISO(d)) ?? 0;
  });
  // Anything already overdue counts toward "today".
  dueForecast[0] = Math.max(dueForecast[0], s.totals.due_now);

  return {
    total: s.totals.cards,
    due: s.totals.due_now,
    learning: s.totals.learning,
    known: s.totals.known,
    streakDays: s.streak.current,
    history,
    dueForecast,
    newWaiting: s.totals.new_waiting,
    bySound: s.totals.by_sound,
  };
}

/** Due card → the fields the review card face renders. */
export interface ReviewCardVM {
  id: number;
  lemma: string;
  reading?: string;
  glosses: string;
  sentence?: string;
  /** English of the card's sentence (cards.translation; the sample deck too). */
  sentenceTranslation?: string;
  /** For tricky words only: register, nuance, traps. Behind "Show explanation". */
  explanation?: string;
  source?: string;
  /** Per-rating next-interval labels (live decks only). */
  intervals?: { again: string; hard: string; good: string; easy: string };
  /** Generated-audio sidecar filename (live decks); presence = "has audio". */
  audioPath?: string;
  /** Captured source image filename (live decks); presence = "has image". */
  imagePath?: string;
  /** What produced the audio, e.g. "voicevox:zundamon". */
  audioSource?: string;
  /** Never-reviewed card — shown revealed on first exposure ("learn" mode). */
  isNew?: boolean;
  /** Dictionary spellings that differ from the (kana) headword — shown on the
   *  back as "also written 拘り" so the kanji stays recognizable without ever
   *  being the form the card teaches. Undefined when the lemma has kanji. */
  alsoWritten?: string[];
  /** Whether the FSRS stability is high enough to collapse recall aids. */
  mature?: boolean;
  /** Cloze cards hide `surface` (else the lemma) inside the sentence on the front. */
  cardType?: 'vocab' | 'cloze';
  surface?: string;
  /** What the cloze front hides, decided by the server. */
  clozeTarget?: string;
  /** The card's mnemonic, structured by the server; rendered, never parsed. */
  mnemonic?: Mnemonic;
  /** The reading split per kanji, from the server. */
  readingParts?: { kanji: string; reading: string }[];
  /** Furigana card that has held for a month. */
  tryKanji?: boolean;
}

export function dueCardToVM(c: DueCard): ReviewCardVM {
  const previewAt = c.preview?.at ?? c.preview_at ?? Date.now() / 1000;
  return {
    id: c.id,
    // Use the curated lemma rather than an alternate dictionary spelling.
    lemma: c.lemma,
    reading: c.readings?.[0],
    alsoWritten: !KANJI_RE.test(c.lemma) && (c.kanji ?? []).some((k) => k !== c.lemma)
      ? (c.kanji ?? []).filter((k) => k !== c.lemma)
      : undefined,
    glosses: glossLine(c.glosses),
    sentence: c.sentence ? stripMarkup(c.sentence) || undefined : undefined,
    sentenceTranslation: c.translation || undefined,
    explanation: c.explanation || undefined,
    source: hostOf(c.source_url),
    audioPath: c.audio_path ?? undefined,
    imagePath: c.image_path ?? undefined,
    audioSource: c.audio_source ?? undefined,
    isNew: c.is_new ?? undefined,
    cardType: c.card_type,
    surface: c.surface ?? undefined,
    clozeTarget: c.cloze_target ?? undefined,
    mnemonic: c.mnemonic ?? undefined,
    readingParts: c.reading_parts ?? undefined,
    tryKanji: c.try_kanji ?? undefined,
    mature: c.mature ?? (c.preview ? c.preview.hard - previewAt > 21 * 86400 : undefined),
    intervals: c.preview
      ? {
          again: formatWhen(c.preview.again, previewAt),
          hard: formatWhen(c.preview.hard, previewAt),
          good: formatWhen(c.preview.good, previewAt),
          easy: formatWhen(c.preview.easy, previewAt),
        }
      : undefined,
  };
}
