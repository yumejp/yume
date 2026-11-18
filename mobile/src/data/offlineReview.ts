import AsyncStorage from '@react-native-async-storage/async-storage';
import { ApiError, type YumeClient } from '../api/client';
import type { DueCard, ReviewRating } from '../api/types';

/**
 * Offline review support: a cached copy of the due queue (so a session can
 * start with the server unreachable) and an outbox of ratings made offline
 * (replayed, in order, when the connection returns).
 *
 * Everything is keyed by the CLIENT IDENTITY (server URL + account) — card
 * ids overlap across accounts, so one account's cached cards or pending
 * ratings must never be replayed against another's deck.
 *
 * Replay leans on the /review_card idempotency token every entry carries: a
 * replay interrupted after the server committed (timeout, app death) resends
 * the same review_id and the server returns the stored result instead of
 * applying FSRS twice. Each entry also carries its original rating time so
 * delayed replay does not distort FSRS intervals or review history.
 */

export interface PendingReview {
  card_id: number;
  rating: ReviewRating;
  review_id: string;
  /** The card's last_review_id when this rating was made ('' = no tokened
   *  reviews) — the optimistic-concurrency expectation. If another device
   *  reviews the card first, the server reports a conflict and this entry is
   *  dropped instead of applied out of order. Consecutive offline ratings of
   *  the same card chain: each expects the previous entry's review_id. */
  prev_review_id: string;
  /** Unix ms when the learner rated. Replayed as the FSRS review time. */
  queued_at: number;
}

const queueKey = (identity: string) => `yume.offline.queue.${identity}`;
const outboxKey = (identity: string) => `yume.offline.outbox.${identity}`;

/** Serialize outbox mutations: the session hook (append/pop) and the
 *  reconnect sync (remove-after-send) touch the same stored array. The lock
 *  covers single read-modify-write steps, never a whole replay — a rating
 *  made mid-replay must not stall behind N in-flight POSTs. */
let chain: Promise<unknown> = Promise.resolve();
function locked<T>(fn: () => Promise<T>): Promise<T> {
  const p = chain.then(fn, fn);
  chain = p.catch(() => {});
  return p;
}

async function readJson<T>(key: string, fallback: T): Promise<T> {
  try {
    const raw = await AsyncStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

// --- Cached due queue -------------------------------------------------------

/** Idle-screen cache warming is throttled per identity: sessions refresh the
 *  cache anyway, so the idle prefetch only needs to cover "opened the app,
 *  didn't study" — not fire on every stats refresh. */
const lastWarmAt = new Map<string, number>();
const WARM_MAX_AGE_MS = 6 * 3600_000;

export function shouldWarmQueue(identity: string): boolean {
  const t = lastWarmAt.get(identity);
  return !t || Date.now() - t > WARM_MAX_AGE_MS;
}

/** Mark up-front (before the fetch) so overlapping idle passes can't
 *  double-fetch. Also used by tests to suppress warming. */
export function markQueueWarmed(identity: string): void {
  lastWarmAt.set(identity, Date.now());
}

export function loadQueue(identity: string): Promise<DueCard[]> {
  return readJson<DueCard[]>(queueKey(identity), []);
}

/** Persist the remaining due queue — called after every live fetch (cache
 *  warming) and after every offline rating (so a killed app resumes where the
 *  session left off). */
export function saveQueue(identity: string, cards: DueCard[]): Promise<void> {
  return locked(() =>
    AsyncStorage.setItem(queueKey(identity), JSON.stringify(cards)).catch(() => {}),
  );
}

// --- Rating outbox ----------------------------------------------------------

export function loadOutbox(identity: string): Promise<PendingReview[]> {
  return readJson<PendingReview[]>(outboxKey(identity), []);
}

function writeOutbox(identity: string, entries: PendingReview[]): Promise<void> {
  return AsyncStorage.setItem(outboxKey(identity), JSON.stringify(entries));
}

export function appendPending(identity: string, entry: PendingReview): Promise<void> {
  return locked(async () => {
    const entries = await loadOutbox(identity);
    entries.push(entry);
    await writeOutbox(identity, entries);
  });
}

/** Undo support: remove the NEWEST entry, but only if it's still the one the
 *  undo targets (review_id match). False = the entry is gone — the reconnect
 *  sync already sent it, so a local pop would silently desync the server. */
export function popPending(identity: string, reviewId: string): Promise<boolean> {
  return locked(async () => {
    const entries = await loadOutbox(identity);
    const last = entries[entries.length - 1];
    if (!last || last.review_id !== reviewId) return false;
    entries.pop();
    await writeOutbox(identity, entries);
    return true;
  });
}

function removePending(identity: string, reviewId: string): Promise<void> {
  return locked(async () => {
    const entries = await loadOutbox(identity);
    await writeOutbox(identity, entries.filter((e) => e.review_id !== reviewId));
  });
}

// --- Replay -----------------------------------------------------------------

export interface ReplayResult {
  sent: number;
  /** Entries the server refused as conflicts (the card was reviewed on
   *  another device first) — dropped, their ratings superseded. */
  skipped: number;
  /** Entries still queued (0 = fully drained). Non-zero after a network
   *  failure mid-replay — the rest waits for the next trigger. */
  remaining: number;
}

/** One replay per identity at a time — the reconnect sync and a session
 *  start can both trigger it; the second caller awaits the first's run.
 *  Keyed by identity: after an account switch, the new account's drain must
 *  not receive another account's in-flight result (whose remaining=0 would
 *  green-light fetching a live queue over the new account's unsent outbox). */
const replayInFlight = new Map<string, Promise<ReplayResult>>();

// Only invalid or deleted ratings are terminal. Authentication, throttling,
// timeouts and server failures can recover without losing the learner's work.
function isTerminalReviewError(err: unknown): boolean {
  return err instanceof ApiError && [400, 404, 410, 422].includes(err.status);
}

export function replayOutbox(client: YumeClient, identity: string): Promise<ReplayResult> {
  const existing = replayInFlight.get(identity);
  if (existing) return existing;
  const run = (async (): Promise<ReplayResult> => {
    const snapshot = await loadOutbox(identity);
    let sent = 0;
    let skipped = 0;
    for (const entry of snapshot) {
      let conflict = false;
      try {
        const res = await client.reviewCard(
          entry.card_id,
          entry.rating,
          entry.review_id,
          entry.prev_review_id ?? undefined,
          entry.queued_at,
        );
        conflict = res.conflict === true;
      } catch (err) {
        // Invalid/deleted ratings cannot recover. Other failures stop replay
        // with the outbox intact, including an expired login or rate limit.
        if (isTerminalReviewError(err)) {
          // EXCEPT a timestamp rejection (bad device clock, chronology):
          // the RATING is still real — resend once letting the server stamp
          // the time, and only drop if that also fails. Losing FSRS timing
          // beats losing the review.
          if (err instanceof ApiError && (err.code === 'reviewed_at_rejected' || /reviewed_at/.test(err.message))) {
            try {
              const res = await client.reviewCard(
                entry.card_id,
                entry.rating,
                entry.review_id,
                entry.prev_review_id ?? undefined,
              );
              await removePending(identity, entry.review_id);
              if (res.conflict === true) skipped++;
              else sent++;
              continue;
            } catch (retryError) {
              if (!isTerminalReviewError(retryError)) {
                const left = await loadOutbox(identity);
                return { sent, skipped, remaining: left.length };
              }
            }
          }
          await removePending(identity, entry.review_id);
          skipped++;
          continue;
        }
        const left = await loadOutbox(identity);
        return { sent, skipped, remaining: left.length };
      }
      await removePending(identity, entry.review_id);
      if (conflict) skipped++;
      else sent++;
    }
    const left = await loadOutbox(identity);
    return { sent, skipped, remaining: left.length };
  })();
  const guarded = run.finally(() => {
    replayInFlight.delete(identity);
  });
  replayInFlight.set(identity, guarded);
  return guarded;
}
