/**
 * Wire types for the yume-server HTTP API. The server is the shared contract.
 * Server quirk to respect everywhere: Go `omitempty` means array fields
 * can be ABSENT, never assume `.length` works without a fallback.
 */

export interface HealthResponse {
  ok: boolean;
  /** Lookups work too (the dictionary loaded); absent on older servers. */
  ready?: boolean;
  version: string;
  /** Git commit the server was built from; empty for an unstamped build. */
  commit?: string;
  dict?: { loaded: boolean; entries: number; error: string };
  schema_version?: number;
  /** HTTP contract level the server speaks; absent before 0.1.0. */
  api_level?: number;
  /** Oldest client API level the server still supports. */
  min_client_api?: number;
  /** Absent on servers older than the sign-in screen (treat as open). In
   *  required mode every deck needs a signed-in account; `signup` says
   *  whether /auth/signup creates one. */
  auth?: { mode: 'open' | 'required'; signup: boolean };
}

/** One server-side snapshot of the deck database (/list_backups). */
export interface BackupInfo {
  name: string;
  /** manual | import | prerestore | premigrate; absent on older servers. */
  kind?: string;
  created_at: string;
  size_bytes: number;
}

export interface ListBackupsResponse {
  backups?: BackupInfo[];
}

/** /create_backup: the new snapshot and any older ones pruned past the cap. */
export interface CreateBackupResponse {
  backup: BackupInfo;
  pruned?: string[];
  max_kept: number;
}

/** /get_settings — per-account client settings blob. `value` is absent until first save. */
export interface GetSettingsResponse {
  value?: Record<string, unknown>;
  updated_at: number;
}

export interface SaveSettingsResponse {
  ok: boolean;
  updated_at: number;
}

/** A user account (server /auth/* responses). email absent for dev accounts. */
export interface AuthUser {
  id: string;
  email?: string;
  display_name: string;
  role: 'owner' | 'user' | 'test';
  created_at: number;
}

/** Response from /auth/signup and /auth/login. */
export interface AuthResponse {
  token: string;
  user: AuthUser;
}

/** A media sidecar (audio/image) as MIME + base64 bytes. */
export interface MediaBlob {
  mime: string;
  data: string;
}

/** Response from /get_card_media. */
export interface GetCardMediaResponse {
  audio?: MediaBlob;
  image?: MediaBlob;
  audio_source?: string;
}

/**
 * One clip attached to a card — metadata only; bytes come from
 * /get_card_media_file. Mirrors Go `srs.CardMedia`. Cards can carry several
 * audio clips (label "word" = the headword alone, label "sentence" = the
 * example sentence). Optional metadata may be absent.
 */
export interface CardMediaInfo {
  id: number;
  card_id: number;
  kind: 'audio' | 'image';
  /** Display label, e.g. "word" | "sentence". */
  label?: string;
  /** What produced the clip, e.g. "voicevox:zundamon". */
  source?: string;
  /** Credited voice name, shown under the audio row with the engine from `source`. */
  voice?: string;
  /** Sidecar filename (server-side only). */
  file: string;
  /** The clip /get_card_media and reveal-autoplay surface. */
  is_primary: boolean;
  /** Unix seconds. */
  created_at: number;
}

/** Response from /list_card_media. */
/** /list_dicts — dictionaries imported on the server (Yomitan format). */
export interface ImportedDict {
  id: number;
  name: string;
  revision?: string;
  author?: string;
  url?: string;
  description?: string;
  enabled: boolean;
}
export interface ListDictsResponse {
  dicts?: ImportedDict[] | null;
}

/** /list_media_voices — every credited voice in the deck with clip counts. */
export interface MediaVoice {
  voice: string;
  source?: string;
  clips: number;
}
export interface ListMediaVoicesResponse {
  voices?: MediaVoice[] | null;
}

export interface ListCardMediaResponse {
  media?: CardMediaInfo[] | null;
}

export interface DeckStats {
  total: number;
  due: number;
  db_path: string;
  /** Due cards past their first review — get_due_cards always returns these. */
  due_reviews?: number;
  /** Due never-reviewed cards — released only when a fetch grants new_limit
   *  (the 🌱 Learn button). due == due_reviews + due_new; show
   *  gatedDueCount(), not raw due. */
  due_new?: number;
  /** Of due_reviews, cards re-due from TODAY's own reviews (an Again'd card
   *  comes back in minutes) — same-day churn, distinguishable from backlog. */
  due_intraday?: number;
  /** Never-reviewed cards waiting to be learned (on request). */
  new_total?: number;
  /** New cards first-reviewed today (spent against today's new-card quota). */
  new_introduced_today?: number;
  /** Present only when a review backlog is being caught up over several days. */
  catch_up?: CatchUp;
  /** Unix time vacation mode started; absent when it is off. */
  vacation_since?: number;
  /** Daily review limit; above it a catch-up plan applies. 0 = no limit. */
  review_limit?: number;
}

/** Today's share of a review backlog; reviews are then served lowest recall first. */
export interface CatchUp {
  backlog: number;
  target_today: number;
  done_today: number;
  remaining_today: number;
  days_left: number;
}

/** A card's memory aid (schema 20): the kanji breakdown and the story. */
export interface MnemonicPart {
  kanji: string;
  meaning: string;
  /** Served only: the part appears in a word this deck marks Known. Absent
   *  for kana parts and on older servers. */
  known?: boolean;
}

export interface Mnemonic {
  parts: MnemonicPart[];
  story: string;
}

export interface DueCard {
  /** The reading split per kanji (供 きょう, 給 きゅう); absent when no honest split exists. */
  reading_parts?: { kanji: string; reading: string }[];
  /** Client-side: when `preview` was computed (unix seconds), stamped on
   *  receipt and on an Again re-queue, so labels measure from that moment. */
  preview_at?: number;
  id: number;
  lemma: string;
  kanji?: string[];
  readings?: string[];
  /** Each inner array is one sense's glosses. */
  glosses?: string[][];
  sentence?: string | null;
  /** English of the sentence, and an explanation for tricky words only;
   *  card columns (they used to ride in an "EN:" note). */
  translation?: string | null;
  explanation?: string | null;
  source_url?: string | null;
  audio_path?: string | null;
  image_path?: string | null;
  /** What produced the audio sidecar, e.g. "voicevox:zundamon". */
  audio_source?: string | null;
  card_type: 'vocab' | 'cloze';
  surface?: string | null;
  mnemonic?: Mnemonic | null;
  /** The mnemonic is the server's shared one, not the card's own. */
  mnemonic_shared?: boolean;
  /** Reviewing with an interval of three weeks or more (server-derived). */
  mature?: boolean;
  /** A furigana card that has held for a month: offer it without the reading. */
  try_kanji?: boolean;
  /** Cloze cards: the text the front hides; omitted when nothing in the sentence matches. */
  cloze_target?: string | null;
  /** True for a never-reviewed card — the client teaches it on first exposure. */
  is_new?: boolean;
  /** Would-be next-review time (unix seconds) per rating — button labels. */
  preview?: { /** When the preview was computed (unix seconds). */ at?: number; again: number; hard: number; good: number; easy: number };
  /** Reserved system tags already on the card (e.g. "flagged",
   *  "needs-explanation") — seeds the overflow-menu toggle state. */
  system_tags?: string[];
  /** review_id of the card's newest review — the version an offline rating
   *  is based on. Sent back as prev_review_id at replay so a rating queued
   *  behind another device's review is skipped, not applied out of order.
   *  Absent when the card has no tokened reviews. */
  last_review_id?: string;
}

export interface DueCardsResponse {
  cards: DueCard[] | null;
}

/** FSRS rating values. 1=Again 2=Hard 3=Good 4=Easy. */
export type ReviewRating = 1 | 2 | 3 | 4;

export interface ReviewResult {
  card_id: number;
  new_due: string;
  new_state: number;
  /** Server auto-graduated this card's word from Learning → Known this review. */
  promoted?: boolean;
  /** Server knocked this card's word Known → Learning (it lapsed) this review. */
  demoted?: boolean;
  /** This lapse switched the card to furigana on the front. */
  furigana?: boolean;
  /** POST-review would-be next-due times (unix seconds) per rating. The
   *  card's fetched preview goes stale the moment a rating commits (an Again
   *  lapse slashes stability), so a same-sitting re-show relabels from this. */
  preview?: { again: number; hard: number; good: number; easy: number };
  /** The prev_review_id expectation didn't match — another device reviewed
   *  the card while this rating was queued. NOTHING was applied; the client
   *  drops the stale rating instead of retrying. */
  conflict?: boolean;
}

/** 0=unknown (implicit), 1=learning, 2=known, 3=ignored. */
export type WordStatusValue = 0 | 1 | 2 | 3;

export interface VocabRow {
  lemma: string;
  status: WordStatusValue;
  card_id: number | null;
  kanji?: string[];
  readings?: string[];
  glosses?: string[][];
  sentence: string | null;
  source_url: string | null;
  created_at: string | null;
  tags?: string[];
  /** Frequency rank and band; only with sort "frequency". */
  rank?: number;
  band?: string;
  mnemonic?: Mnemonic | null;
}

export interface VocabBand {
  band: string;
  label: string;
  /** Ranked words in the band across the dictionary; 0 = open-ended. */
  size?: number;
  /** Words you track in the band. */
  total: number;
  known: number;
  learning: number;
  unknown: number;
  ignored: number;
}

export interface VocabBandsResponse {
  bands: VocabBand[];
}

export interface VocabFilter {
  status?: WordStatusValue;
  has_card?: boolean;
  search?: string;
  tags?: string[];
  /** "frequency" orders most common first and fills rank/band. */
  sort?: 'newest' | 'frequency';
  /** true → cards that have never been reviewed. */
  new?: boolean;
  /** Frequency band key ("1k", "3k", "6k", "12k", "beyond", "unranked"). */
  band?: string;
}

export interface ListVocabResponse {
  total: number;
  rows: VocabRow[] | null;
}

/** One Tanaka-corpus example sentence with its English translation. */
export interface ExampleSentence {
  text: string;
  translation: string;
}

export interface ExamplesResponse {
  examples: ExampleSentence[] | null;
}

/** kagome token. `reading` is katakana; absent for punctuation/symbols. */
export interface Token {
  surface: string;
  reading?: string;
  lemma?: string;
  pos?: string;
  pos1?: string;
}

export interface TokenizeResponse {
  tokens: Token[] | null;
}

export interface DictSense {
  pos?: string[];
  glosses?: string[];
}

export interface DictEntry {
  /** Language of the glosses: 'en' for JMdict, the dictionary's for imported entries. */
  lang?: string;
  id: number;
  kanji?: string[];
  readings?: string[];
  senses: DictSense[];
  pitches?: (number | null)[];
  frequency?: number | null;
  /** JMdict commonness: 1–48 = nf band (×500 ≈ rank in the top 24k),
   *  60 = ichi1/news1-class, 80 = the *2 classes; absent = unranked. */
  priority?: number;
  /** Some form carries ichi1 or news1; outranks the nf band in `priority`. */
  common?: boolean;
}

export interface LookupResult {
  query: string;
  entries: DictEntry[];
}

export interface LookupResponse {
  results: LookupResult[] | null;
}

export interface WordStatusRow {
  lemma: string;
  status: WordStatusValue;
  updated_at?: string;
}

export interface WordStatusesResponse {
  statuses: WordStatusRow[] | null;
}

/** An AI-tutor suggestion (watch word), joined with the learner's status. */
export interface WatchWord {
  lemma: string;
  note?: string;
  source?: string;
  created_at?: string;
  status: WordStatusValue;
  /** Dictionary readings (server-attached, best-effort) — furigana source. */
  readings?: string[];
}

/** One recurring unknown found by scanning pasted Japanese text. */
export interface MinedWord {
  lemma: string;
  count: number;
  status: WordStatusValue;
  sentence?: string;
  readings?: string[];
  glosses?: string[];
}

export interface MineTranscriptResponse {
  words: MinedWord[] | null;
  sentences: number;
}

export interface WatchWordsResponse {
  words: WatchWord[] | null;
}

export interface SaveCardRequest {
  jmdict_id?: number;
  lemma: string;
  kanji?: string[];
  readings?: string[];
  glosses?: string[][];
  sentence?: string;
  source_url?: string;
}

export interface SaveCardResponse {
  id: number;
  /** False when the deck already held this jmdict_id — nothing was added. */
  created: boolean;
}

/** /card_status — which dictionary entries / lemmas are already saved.
 *  The lemma check catches cards saved WITHOUT a jmdict_id (third-party SRS
 *  imports, mined words), which the id check alone can't see. */
export interface CardStatusResponse {
  statuses?: { jmdict_id: number; card_id: number | null }[];
  lemma_statuses?: { lemma: string; card_id: number | null }[];
}

/** What a note is; the server sets it. The card's own sentence,
 *  translation and explanation are card fields, not notes. */
export type CardNoteKind = 'mnemonic' | 'tutor' | 'example' | 'triage';

export interface CardNote {
  id: number;
  card_id: number;
  kind: CardNoteKind;
  note: string;
  /** The example's translation (kind example only). */
  translation?: string;
  created_at: string;
}

export interface CardNotesResponse {
  notes: CardNote[] | null;
}

export interface KanjiWord {
  card_id: number;
  lemma: string;
  readings?: string[];
  /** 0=unknown 1=learning 2=known 3=ignored */
  status: number;
}

/** KANJIDIC2 facts; absent when the server has no kanji dictionary installed. */
export interface KanjiDictEntry {
  kanji: string;
  meanings?: string[];
  on?: string[];
  kun?: string[];
  grade?: number;
  strokes?: number;
  jlpt?: number;
  freq?: number;
}

/** /kanji_info: dictionary facts, the tutor's story (if any) and the deck
 *  words using the kanji. */
/** One part a kanji is explained through, with its own story. */
export interface KanjiComponent {
  kanji: string;
  meaning: string;
  note?: string;
  note_shared?: boolean;
  known?: boolean;
}

export interface KanjiInfo {
  /** The story is the server's shared one, not this deck's own. */
  note_shared?: boolean;
  kanji: string;
  note?: string;
  /** The kanji appears in a word this deck marks Known. */
  known?: boolean;
  components?: KanjiComponent[];
  words: KanjiWord[];
  dict?: KanjiDictEntry;
}

export interface DayCount {
  date: string; // "YYYY-MM-DD" in the LEARNER's zone (client sends tz_offset_minutes)
  count: number;
}

export interface CumulativePoint {
  date: string;
  cumulative: number;
}

export interface DashboardStats {
  totals: {
    known: number;
    learning: number;
    ignored: number;
    cards: number;
    /** Studied cards due now; never-seen cards are in new_waiting. */
    due_now: number;
    /** Never-seen cards waiting for Learn new words. Older servers omit it
     *  and count those cards in due_now. */
    new_waiting?: number;
    /** Learning words on furigana cards: known by sound, not by kanji. */
    by_sound?: number;
  };
  streak: { current: number; longest: number };
  this_week: { known_added: number; reviews: number; mined: number };
  /** Sparse — only days with activity. */
  heatmap: DayCount[] | null;
  mining_curve: CumulativePoint[] | null;
  /** Reviews due on each of the next 14 days (never-seen cards excluded). */
  due_forecast: DayCount[] | null;
  total_reviews: number;
  /** Reviews since the learner's midnight. Requires tz_offset_minutes. */
  reviews_today?: number;
  milestone: { metric: string; target: number; have: number };
}
