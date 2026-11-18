import { reportUnauthorized, reportUnreachable } from '../lib/connectivity';
import type {
  KanjiInfo,
  VocabBand,
  VocabBandsResponse,
  AuthResponse,
  AuthUser,
  BackupInfo,
  CreateBackupResponse,
  ListBackupsResponse,
  DashboardStats,
  DeckStats,
  DueCard,
  CardMediaInfo,
  CardNote,
  CardNotesResponse,
  DictEntry,
  DueCardsResponse,
  ExampleSentence,
  ExamplesResponse,
  GetCardMediaResponse,
  GetSettingsResponse,
  HealthResponse,
  ImportedDict,
  ListCardMediaResponse,
  ListDictsResponse,
  ListMediaVoicesResponse,
  MediaVoice,
  MnemonicPart,
  ListVocabResponse,
  LookupResponse,
  MediaBlob,
  LookupResult,
  ReviewRating,
  ReviewResult,
  SaveCardRequest,
  CardStatusResponse,
  SaveCardResponse,
  SaveSettingsResponse,
  Token,
  TokenizeResponse,
  VocabFilter,
  MineTranscriptResponse,
  WatchWord,
  WatchWordsResponse,
  WordStatusRow,
  WordStatusesResponse,
  WordStatusValue,
} from './types';

/**
 * Minimal client for the yume-server HTTP API. Every method is a
 * `POST /<method>` with a JSON body. Base URL is wherever yume-server is reachable, e.g.
 * `http://your-server:7654`.
 */
export interface YumeClientOptions {
  /** Session bearer token; sent as Authorization on every request. */
  token?: string | null;
  /** Dev-only X-Yume-User override (server open mode); ignored once a token is set. */
  devUser?: string | null;
}

/** HTTP-level failure with the status code attached, so callers can react to
 *  the code (e.g. 401 → drop the session) instead of matching message text. */
export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly code: string = '') {
    super(message);
    this.name = 'ApiError';
  }
}

/** True for failures that mean "the server never answered" (fetch rejected,
 *  request timed out) rather than "the server said no". Classified by error
 *  type, never message text. */
export function isNetworkError(e: unknown): boolean {
  if (e instanceof ApiError || !(e instanceof Error)) return false;
  return e.name === 'AbortError' || e.name === 'TimeoutError' || e instanceof TypeError;
}

export class YumeClient {
  private token: string | null;
  private devUser: string | null;

  constructor(private baseUrl: string, opts: YumeClientOptions = {}) {
    this.token = opts.token ?? null;
    this.devUser = opts.devUser ?? null;
  }

  private headers(json: boolean): Record<string, string> {
    const h: Record<string, string> = {};
    if (json) h['Content-Type'] = 'application/json';
    // A real session always wins; the dev header is only meaningful in the
    // server's open mode and is ignored once a bearer token is present.
    if (this.token) h['Authorization'] = `Bearer ${this.token}`;
    else if (this.devUser) h['X-Yume-User'] = this.devUser;
    return h;
  }

  private request<T>(
    httpMethod: 'GET' | 'POST',
    method: string,
    params: unknown,
    timeoutMs: number,
  ): Promise<T> {
    return this.send(httpMethod, method, params, timeoutMs, (resp) => resp.json() as Promise<T>);
  }

  /** One HTTP round trip; `read` consumes the body inside the timeout. */
  private async send<T>(
    httpMethod: 'GET' | 'POST',
    method: string,
    params: unknown,
    timeoutMs: number,
    read: (resp: Response) => Promise<T>,
  ): Promise<T> {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeoutMs);
    try {
      const resp = await fetch(`${this.baseUrl.replace(/\/+$/, '')}/${method}`, {
        method: httpMethod,
        headers: this.headers(httpMethod === 'POST'),
        body: httpMethod === 'POST' ? JSON.stringify(params ?? {}) : undefined,
        signal: ctrl.signal,
      });
      if (!resp.ok) {
        const { message, code } = await errorDetail(resp, method);
        // A 401 from sign-in itself is a wrong password, not an ended session.
        if (resp.status === 401 && !method.startsWith('auth/')) reportUnauthorized(this.token);
        throw new ApiError(message, resp.status, code);
      }
      return await read(resp);
    } catch (err) {
      if (isNetworkError(err)) reportUnreachable();
      throw err;
    } finally {
      clearTimeout(timer);
    }
  }

  /** The deck's language, sent on every call so the server scopes the
   *  request (schema 19 keys every lemma table by lang). */
  lang = 'ja';

  private call<T>(method: string, params: unknown, timeoutMs = 10000): Promise<T> {
    const body = params && typeof params === 'object' && !Array.isArray(params)
      ? { lang: this.lang, ...(params as Record<string, unknown>) }
      : params;
    return this.request<T>('POST', method, body, timeoutMs);
  }

  health(): Promise<HealthResponse> {
    return this.call<HealthResponse>('health', {}, 5000);
  }

  /** Turns vacation mode on or off; ending it moves due dates by the time away. */
  setVacation(on: boolean): Promise<{ vacation_since: number; shifted: number }> {
    return this.call('vacation', { on });
  }

  /** Sets the daily review limit (0 = no limit). */
  setReviewLimit(limit: number): Promise<{ review_limit: number }> {
    return this.call('set_review_limit', { limit });
  }

  private statsInFlight: Promise<DeckStats> | null = null;

  /** Callers that ask while a fetch is in flight share it: several screens
   *  read deck stats at once, and on a slow link their queue made other
   *  requests time out. */
  deckStats(): Promise<DeckStats> {
    if (this.statsInFlight) return this.statsInFlight;
    const p = this.call<DeckStats>('deck_stats', { tz_offset_minutes: tzOffsetMinutes() })
      .finally(() => { if (this.statsInFlight === p) this.statsInFlight = null; });
    this.statsInFlight = p;
    return p;
  }

  /** Server-side copy of the app settings for this account — the cross-device
   *  source of truth (device storage is just a cache; phone browsers purge it). */
  getSettings(): Promise<GetSettingsResponse> {
    return this.call<GetSettingsResponse>('get_settings', { key: 'mobile' });
  }

  saveSettings(value: Record<string, unknown>): Promise<SaveSettingsResponse> {
    return this.call<SaveSettingsResponse>('save_settings', { key: 'mobile', value });
  }

  /** newLimit caps how many never-reviewed cards are introduced today (the rest
   *  stay pending). Omit / -1 to disable the cap. */
  async getDueCards(limit = 20, newLimit?: number): Promise<DueCard[]> {
    const body: Record<string, number> = { limit, tz_offset_minutes: tzOffsetMinutes() };
    if (newLimit != null) body.new_limit = newLimit;
    const resp = await this.call<DueCardsResponse>('get_due_cards', body);
    return resp.cards ?? [];
  }

  /** reviewId is an idempotency token: resending the same one (retry after a
   *  timed-out-but-committed POST) replays the stored result instead of
   *  applying FSRS twice. prevReviewId (offline replay only) is the card's
   *  last_review_id as fetched ('' = none) — on mismatch the server applies
   *  nothing and returns conflict:true. reviewedAtMs preserves the time an
   *  offline rating was made; the server accepts it only with prevReviewId. */
  reviewCard(
    cardId: number,
    rating: ReviewRating,
    reviewId?: string,
    prevReviewId?: string,
    reviewedAtMs?: number,
  ): Promise<ReviewResult> {
    const body: Record<string, unknown> = { card_id: cardId, rating };
    if (reviewId) body.review_id = reviewId;
    if (prevReviewId !== undefined) body.prev_review_id = prevReviewId;
    if (reviewedAtMs !== undefined) body.reviewed_at = Math.floor(reviewedAtMs / 1000);
    return this.call<ReviewResult>('review_card', body);
  }

  /** Undo the single most recent review (restores FSRS state server-side).
   *  Also returns the undone review's idempotency token ('' for pre-token
   *  rows) so the caller can verify WHICH review was reverted — the server
   *  undoes the newest in the whole log, which may be another device's. */
  async undoReview(): Promise<{ cardId: number; reviewId: string }> {
    const resp = await this.call<{ card_id: number; undone_review_id?: string }>('undo_review', {
      tz_offset_minutes: tzOffsetMinutes(),
    });
    return { cardId: resp.card_id, reviewId: resp.undone_review_id ?? '' };
  }

  /** Delete one card (its notes + media cascade). The lemma's word_status
   *  survives — removing a flashcard doesn't un-know the word. */
  async deleteCard(cardId: number): Promise<boolean> {
    const resp = await this.call<{ deleted: boolean }>('delete_card', { card_id: cardId });
    return resp.deleted;
  }

  /** Apply (or, with remove=true, clear) a reserved system tag on a card —
   *  e.g. 'flagged' when the learner spots a problem. Server validates the tag. */
  async tagCard(cardId: number, tag: string, remove = false): Promise<void> {
    await this.call<{ ok: boolean }>('tag_card', { card_id: cardId, tag, remove });
  }

  stats(): Promise<DashboardStats> {
    return this.call<DashboardStats>('stats', { tz_offset_minutes: tzOffsetMinutes() });
  }

  listVocab(filter: VocabFilter, limit = 100, offset = 0): Promise<ListVocabResponse> {
    return this.call<ListVocabResponse>('list_vocab', { filter, limit, offset });
  }

  async vocabBands(filter: VocabFilter): Promise<VocabBand[]> {
    const resp = await this.call<VocabBandsResponse>('vocab_bands', { filter });
    return resp.bands ?? [];
  }

  /** Tanaka-corpus example sentences (with English) for the given headwords.
   *  With a reading, sentences the corpus reads differently rank last. */
  async examples(queries: string[], limit = 3, lang = 'ja', reading?: string): Promise<ExampleSentence[]> {
    const resp = await this.call<ExamplesResponse>('examples', { queries, lang, limit, reading });
    return resp.examples ?? [];
  }

  /** Tokenize a sentence (kagome) — surfaces, readings, lemmas, POS. */
  async tokenize(text: string, lang = 'ja'): Promise<Token[]> {
    const resp = await this.call<TokenizeResponse>('tokenize', { text, lang });
    return resp.tokens ?? [];
  }

  /** Dictionary lookup. Pass [lemma, surface] to merge across both forms. */
  async lookup(queries: string[], lang = 'ja'): Promise<LookupResult[]> {
    const resp = await this.call<LookupResponse>('lookup', { queries, lang });
    return resp.results ?? [];
  }

  /** Reverse (English → Japanese) dictionary search, ranked best-first. */
  async searchEnglish(query: string, limit = 50, lang = 'ja'): Promise<DictEntry[]> {
    const resp = await this.call<{ entries: DictEntry[] | null }>('search_english', {
      query,
      limit,
      lang,
    });
    return resp.entries ?? [];
  }

  /** Per-lemma word statuses; only lemmas with an explicit row are returned. */
  async getWordStatuses(lemmas: string[]): Promise<WordStatusRow[]> {
    const resp = await this.call<WordStatusesResponse>('get_word_statuses', { lemmas });
    return resp.statuses ?? [];
  }

  /** Set a lemma's status. status=0 (unknown) clears the row server-side. */
  async setWordStatus(lemma: string, status: WordStatusValue): Promise<void> {
    await this.call('set_word_status', { lemma, status });
  }

  /** AI-tutor suggestions (watch words) with the learner's status for each. */
  async listWatchWords(): Promise<WatchWord[]> {
    const resp = await this.call<WatchWordsResponse>('list_watch_words', {});
    return resp.words ?? [];
  }

  /** Withdraw one suggestion from the learner's watchlist. */
  async removeWatchWord(lemma: string): Promise<void> {
    await this.call('remove_watch_word', { lemma });
  }

  /** Scan Japanese text for recurring unknown words. Only the
   *  text travels; the server stores nothing from this call. */
  async mineTranscript(
    text: string,
    lang = 'ja',
    opts: { minCount?: number } = {},
  ): Promise<MineTranscriptResponse> {
    return this.call<MineTranscriptResponse>('mine_transcript', {
      text,
      lang,
      ...(opts.minCount != null ? { min_count: opts.minCount } : {}),
    });
  }

  /** Turn accepted words from a text scan into suggestions (the HTTP
   *  twin of the tutor's suggest_words). */
  async addWatchWords(words: { lemma: string; note?: string }[]): Promise<number> {
    const resp = await this.call<{ added: number }>('add_watch_words', { words });
    return resp.added;
  }

  /** Save a card to the review deck. */
  async saveCard(req: SaveCardRequest): Promise<SaveCardResponse> {
    return this.call<SaveCardResponse>('save_card', req);
  }

  /** Which of these entries/lemmas already have a card — the "already in
   *  deck" check behind the dictionary's save button. */
  async cardStatus(jmdictIds: number[], lemmas: string[]): Promise<CardStatusResponse> {
    return this.call<CardStatusResponse>('card_status', { jmdict_ids: jmdictIds, lemmas });
  }

  /** Notes attached to a card (e.g. 🎓 AI-tutor usage explanations). */
  kanjiInfo(kanji: string): Promise<KanjiInfo> {
    return this.call<KanjiInfo>('kanji_info', { kanji });
  }

  async listCardNotes(cardId: number): Promise<CardNote[]> {
    const resp = await this.call<CardNotesResponse>('list_card_notes', { card_id: cardId });
    return resp.notes ?? [];
  }

  /** Attach a note of an explicit kind. The server refuses mnemonics and
   *  translations here: those are card fields (setMnemonic, updateCard). */
  addCardNote(
    cardId: number,
    kind: 'tutor' | 'example',
    note: string,
    translation?: string,
  ): Promise<{ id: number; created_at: string }> {
    return this.call<{ id: number; created_at: string }>('add_card_note', {
      card_id: cardId,
      kind,
      note,
      ...(translation ? { translation } : {}),
    });
  }

  /** Edit a card's own fields in place; only the fields given are written,
   *  and an empty string clears one. */
  async updateCard(cardId: number, fields: { translation?: string; explanation?: string }): Promise<void> {
    await this.call<{ ok: boolean }>('update_card', { card_id: cardId, ...fields });
  }

  /** Replace the card's own mnemonic (an override of the shared one). The
   *  server validates parts against the lemma; empty parts are allowed. */
  async setMnemonic(cardId: number, parts: MnemonicPart[], story: string): Promise<boolean> {
    const resp = await this.call<{ replaced: boolean }>('set_mnemonic', { card_id: cardId, parts, story });
    return resp.replaced;
  }

  /** A card's media sidecars (audio/image) as base64 blobs + the audio source. */
  getCardMedia(cardId: number): Promise<GetCardMediaResponse> {
    return this.call<GetCardMediaResponse>('get_card_media', { card_id: cardId });
  }

  /** Every clip on a card (metadata only, no bytes), audio first then image.
   *  Clips without labels are also supported. */
  async listCardMedia(cardId: number): Promise<CardMediaInfo[]> {
    const resp = await this.call<ListCardMediaResponse>('list_card_media', { card_id: cardId });
    return resp.media ?? [];
  }

  /** Dictionaries imported on the server, for the About credit and to know
   *  whether Japanese definitions exist at all. */
  async listDicts(): Promise<ImportedDict[]> {
    const resp = await this.call<ListDictsResponse>('list_dicts', {});
    return resp.dicts ?? [];
  }

  /** Every credited voice across the deck (VOICEVOX names) with clip counts,
   *  for the About sheet's audio credit. */
  async listMediaVoices(): Promise<MediaVoice[]> {
    const resp = await this.call<ListMediaVoicesResponse>('list_media_voices', {});
    return resp.voices ?? [];
  }

  /** One clip's bytes by media id. The wire returns the MediaBlob directly
   *  ({mime, data}) — NOT wrapped in {audio: …} like get_card_media. */
  getCardMediaFile(mediaId: number): Promise<MediaBlob> {
    return this.call<MediaBlob>('get_card_media_file', { media_id: mediaId });
  }

  // --- Your data -------------------------------------------------------------

  /** The whole deck as an Anki package (.apkg): cards, scheduling, review
   *  history, tags and media. The one binary response in the API. */
  exportApkg(): Promise<Blob> {
    return this.send('POST', 'export_apkg', { lang: this.lang }, 120_000, (resp) => resp.blob());
  }

  /** Server-side deck snapshots, newest first. Managed hosts refuse this
   *  (403), which is how the app knows to hide snapshots. */
  async listBackups(): Promise<BackupInfo[]> {
    const resp = await this.call<ListBackupsResponse>('list_backups', {});
    return resp.backups ?? [];
  }

  /** Snapshot the deck now; the server keeps `max_kept` and prunes the oldest. */
  createBackup(label = 'app'): Promise<CreateBackupResponse> {
    return this.call<CreateBackupResponse>('create_backup', { label }, 60_000);
  }

  // --- Auth -----------------------------------------------------------------

  /** Register a new account; returns a session token + the user. */
  signup(email: string, password: string, displayName?: string): Promise<AuthResponse> {
    return this.call<AuthResponse>('auth/signup', { email, password, display_name: displayName });
  }

  /** Log in with email + password; returns a session token + the user. */
  login(email: string, password: string): Promise<AuthResponse> {
    return this.call<AuthResponse>('auth/login', { email, password });
  }

  /** Revoke the current session token (best-effort). */
  async logout(): Promise<void> {
    await this.call('auth/logout', {});
  }

  /** Resolve the current token to its account; throws on 401 (invalid/expired). */
  async me(): Promise<AuthUser> {
    const resp = await this.request<{ user: AuthUser }>('GET', 'auth/me', null, 5000);
    return resp.user;
  }
}

/** Fresh idempotency token for reviewCard. Opaque and collision-unlikely is
 *  all it needs to be (not cryptographic) — server accepts [A-Za-z0-9._:-]. */
export function newReviewId(): string {
  return `${Date.now().toString(36)}.${Math.random().toString(36).slice(2, 12)}`;
}

/** The device's offset in minutes EAST of UTC (e.g. JST = +540). Sent on the
 *  day-boundary-sensitive calls so the daily new-card quota and the undo
 *  "today" bound reset at the LEARNER's midnight, not the server's — the
 *  server and the learner may sit in different timezones. */
function tzOffsetMinutes(): number {
  return -new Date().getTimezoneOffset();
}

/** Surface the server's { error } JSON cleanly; fall back to status text. */
async function errorDetail(resp: Response, method: string): Promise<{ message: string; code: string }> {
  const text = await resp.text().catch(() => '');
  try {
    const body = JSON.parse(text) as { error?: string; code?: string };
    if (body?.error) return { message: body.error, code: body.code ?? '' };
  } catch {
    /* not JSON */
  }
  return { message: `${method} ${resp.status}${text ? `: ${text.slice(0, 200)}` : ''}`, code: '' };
}
