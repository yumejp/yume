/** Card and vocabulary view models derived from the server API. */

export type WordStatus = 'unknown' | 'learning' | 'known' | 'ignored';

export interface Card {
  id: number;
  lemma: string;
  /** Kana reading; absent for non-JP or kana-only lemmas. */
  reading?: string;
  glosses: string[];
  /** The mined sentence the word came from. */
  sentence?: string;
  sentenceTranslation?: string;
  /** Human-readable provenance, e.g. "Sample · conversation". */
  source?: string;
  tags: string[];
  status: WordStatus;
  /** Days until due. Negative = overdue, 0 = due today. */
  dueInDays: number;
  /** Frequency band key from the server's frequency sort (Library tiles). */
  band?: string;
}

export interface DayActivity {
  /** ISO date (YYYY-MM-DD). */
  date: string;
  reviews: number;
}

export interface DeckStats {
  total: number;
  due: number;
  learning: number;
  known: number;
  streakDays: number;
  /** Reviews per day, oldest first — feeds the heatmap. */
  history: DayActivity[];
  /** Cards coming due over the next 7 days, index 0 = today. */
  dueForecast: number[];
  /** Never-seen cards waiting to be learned; not part of `due`. */
  newWaiting?: number;
  /** Learning words on furigana cards. */
  bySound?: number;
}
