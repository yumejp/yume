import { useCallback, useEffect, useRef, useState } from 'react';
import type { CatchUp, DashboardStats, DueCard, ReviewRating } from '../api/types';
import { newReviewId } from '../api/client';
import { invalidate, useInvalidationVersion } from '../data/queryStore';
import { errorText, isNetworkError } from '../lib/errors';
import { dueCardToVM, gatedDueCount, type ReviewCardVM } from '../data/live';
import {
  appendPending,
  loadOutbox,
  loadQueue,
  markQueueWarmed,
  popPending,
  replayOutbox,
  saveQueue,
  shouldWarmQueue,
} from '../data/offlineReview';
import { SAMPLE_DUE_QUEUE } from '../data/sample';
import type { Card } from '../data/types';
import { useServer } from '../server/ServerContext';
import { useSettings } from '../settings/SettingsContext';
export type Rating = 'again' | 'hard' | 'good' | 'easy';

export const RATINGS: { key: Rating; label: string; value: ReviewRating }[] = [
  { key: 'again', label: 'Again', value: 1 },
  { key: 'hard', label: 'Hard', value: 2 },
  { key: 'good', label: 'Good', value: 3 },
  { key: 'easy', label: 'Easy', value: 4 },
];

export type Phase = 'idle' | 'loading' | 'front' | 'back' | 'done';

/** Receipt for the last rating: "Rated Good · 慣れる → Known". `grad` is the
 *  server's auto-promotion/demotion of that word on this review. */
export type RatingReceipt = {
  label: string;
  lemma: string;
  grad: 'promoted' | 'demoted' | null;
};

/** Session snapshot taken just before one rating — the unit of the undo
 *  stack. The server keeps its own per-review snapshots on review_log. */
interface UndoSnapshot {
  liveQueue: DueCard[];
  sampleQueue: Card[];
  doneCount: number;
  againCount: number;
  /** Idempotency token of the rating this snapshot precedes. Offline: names
   *  the outbox entry to pop. Live: lets undo verify the server reverted THIS
   *  rating, not one another device committed since. */
  reviewId?: string;
}

/** Re-enter an Again'd card at a random slot ≥ MIN_AGAIN_GAP ahead (uniform
 *  over the tail) so the failure pile never replays in fail order. Shorter
 *  queues just append. */
const MIN_AGAIN_GAP = 3;
export function reinsertAgain<T>(rest: T[], card: T): T[] {
  const lo = Math.min(MIN_AGAIN_GAP, rest.length);
  const at = lo + Math.floor(Math.random() * (rest.length - lo + 1));
  return [...rest.slice(0, at), card, ...rest.slice(at)];
}

function sampleToVM(c: Card): ReviewCardVM {
  return {
    id: c.id,
    lemma: c.lemma,
    reading: c.reading,
    glosses: c.glosses.join(' · '),
    sentence: c.sentence,
    sentenceTranslation: c.sentenceTranslation,
    source: c.source,
  };
}

/**
 * The review-session state machine: queue management, rating writes,
 * multi-step undo, card removal, new-card gating and the session-complete
 * flow. The server owns scheduling, and failed writes do not advance the queue.
 */
export type ReviewSession = ReturnType<typeof useReviewSession>;

export function useReviewSession() {
  const server = useServer();
  const { settings } = useSettings();
  // 'checking' counts as live so a re-probe doesn't flip screens to sample mode.
  const live = (server.status === 'online' || server.status === 'checking') && server.client != null;
  const identity = server.identity;

  const [phase, setPhase] = useState<Phase>('idle');
  // Fixed at start(). Must survive connection flaps — a live session with the
  // network down shows rating errors, never dumps the queue. 'offline' runs on
  // the CACHED due queue with ratings written to the outbox.
  const [mode, setMode] = useState<'live' | 'sample' | 'offline' | null>(null);
  const [liveQueue, setLiveQueue] = useState<DueCard[]>([]);
  const [sampleQueue, setSampleQueue] = useState<Card[]>([]);
  const [dueCount, setDueCount] = useState<number | null>(null);
  const [doneCount, setDoneCount] = useState(0);
  const [againCount, setAgainCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [moreDue, setMoreDue] = useState(0);
  // From the same stats as dueCount/moreDue, so the count and its label change together.
  const [catchUp, setCatchUp] = useState<CatchUp | null>(null);
  // New words behind the on-request gate — the 🌱 button is their only way in.
  const [newPending, setNewPending] = useState(0);
  const [offlineCount, setOfflineCount] = useState(0);
  const [pendingSync, setPendingSync] = useState(0);
  const [syncNotice, setSyncNotice] = useState<string | null>(null);
  // False while the done screen's follow-up fetches are in flight; the CTA row
  // hides behind it so buttons don't morph as counts land.
  const [doneReady, setDoneReady] = useState(true);
  // True while start()'s fetches are in flight; the done screen keeps
  // rendering (spinner in place of CTAs) during a "Keep going" start.
  const [starting, setStarting] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  // Epoch: async work captures it at entry and drops its writes if it moved
  // (client switch, End, restart, undo). Card ids overlap across accounts, so
  // a late fetch landing under another client would reschedule the wrong deck.
  const epochRef = useRef(0);
  // Synchronous mirrors of `starting` / `submitting`: a fast second tap lands
  // before React re-renders the disabled control; the refs are set before any
  // await so the second tap sees them.
  const startingRef = useRef(false);
  const submittingRef = useRef(false);
  const beginSubmit = () => {
    submittingRef.current = true;
    setSubmitting(true);
  };
  const endSubmit = () => {
    submittingRef.current = false;
    setSubmitting(false);
  };
  const [pendingRating, setPendingRating] = useState<Rating | null>(null);
  // Undo stack, newest last. Cleared whenever the captured queues stop being
  // restorable (start, delete, client switch).
  const [undoStack, setUndoStack] = useState<UndoSnapshot[]>([]);
  const UNDO_STACK_MAX = 100;
  // Pending review idempotency token (see rate()); survives a failed POST so
  // the retry replays instead of double-applying.
  const reviewTokenRef = useRef<{ cardId: number; rating: ReviewRating; token: string } | null>(null);
  const [idleStats, setIdleStats] = useState<DashboardStats | null>(null);
  // Detects the sample→live crossing so the sample count never poses as live.
  const wasLiveRef = useRef(false);
  const [lastRated, setLastRated] = useState<RatingReceipt | null>(null);
  // Non-error status line ("Connection lost. Answers are saved…"); self-clears.
  const [notice, setNotice] = useState<string | null>(null);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(null), 6000);
    return () => clearTimeout(t);
  }, [notice]);

  useEffect(() => {
    if (!lastRated) return;
    const t = setTimeout(() => setLastRated(null), lastRated.grad ? 5000 : 4000);
    return () => clearTimeout(t);
  }, [lastRated]);

  // Idle- and done-screen counts. The due count is GATED (gatedDueCount) —
  // raw deck_stats.due includes capped new cards. Subscribed to 'stats' but
  // guarded to those screens: every rating bumps 'stats' too, and refetching
  // mid-session would cost a round-trip per card. The done screen must follow
  // the tab badge: cards keep coming due while it sits there.
  const statsVersion = useInvalidationVersion('stats');
  useEffect(() => {
    if (!live) {
      wasLiveRef.current = false;
      setIdleStats(null);
      // Disconnected idle: offer the cached queue; sample deck only when
      // nothing is cached for this account.
      let cancelled = false;
      Promise.all([loadQueue(identity), loadOutbox(identity)])
        .then(([cached, outbox]) => {
          if (cancelled) return;
          setOfflineCount(cached.length);
          setPendingSync(outbox.length);
          setDueCount(cached.length > 0 ? cached.length : SAMPLE_DUE_QUEUE.length);
        })
        .catch(() => { if (!cancelled) setDueCount(SAMPLE_DUE_QUEUE.length); });
      return () => { cancelled = true; };
    }
    if (phase !== 'idle' && phase !== 'loading' && phase !== 'done') return;
    // Crossing sample → live: the standing dueCount is the sample queue's
    // length and must not wear the live label while the fetch runs.
    if (!wasLiveRef.current) {
      wasLiveRef.current = true;
      setDueCount(null);
    }
    let cancelled = false;
    server.client!.deckStats()
      .then((s) => {
        if (cancelled) return;
        if (phase === 'done') setMoreDue(gatedDueCount(s));
        else setDueCount(gatedDueCount(s));
        setCatchUp(s.catch_up ?? null);
        setNewPending(s.new_total ?? 0);
      })
      .catch(() => { if (!cancelled && phase !== 'done') setDueCount(null); });
    if (phase === 'done') return () => { cancelled = true; };
    if (phase === 'idle') {
      server.client!.stats()
        .then((s) => { if (!cancelled) setIdleStats(s); })
        .catch(() => { /* kicker is optional */ });
    }
    // Warm the offline cache (idle only — 'loading' is already fetching this
    // queue). Skipped while the outbox holds unsent ratings: the server's due
    // list does not reflect them, so it cannot be cached safely.
    if (phase === 'idle') {
      const warmClient = server.client!;
      loadOutbox(identity)
        .then((outbox) => {
          if (cancelled) return;
          setPendingSync(outbox.length);
          if (outbox.length > 0 || !shouldWarmQueue(identity)) return;
          markQueueWarmed(identity);
          return warmClient.getDueCards(20, 0).then((cards) => {
            if (cancelled) return;
            setOfflineCount(cards.length);
            return saveQueue(identity, cards);
          });
        })
        .catch(() => { /* cache warming is best-effort */ });
    }
    return () => { cancelled = true; };
  }, [live, server.client, phase, statsVersion, identity]);

  // Reset when client identity changes because card ids are scoped per user.
  // Connection status changes do not invalidate the active session.
  useEffect(() => {
    epochRef.current += 1;
    reviewTokenRef.current = null;
    setPhase('idle');
    setMode(null);
    setLiveQueue([]);
    setSampleQueue([]);
    setError(null);
    setUndoStack([]);
    setLastRated(null);
  }, [server.client]);

  // Shared start path. Callers hold the `starting` latch; this releases it.
  // `newLimit`: 0 for a plain start (new words are on-request only), or the
  // Learn button's grant ceiling.
  const startInner = useCallback(async (newLimit: number, fromDone: boolean) => {
    const epoch = ++epochRef.current;
    // fromDone starts stay on the done screen; counters reset right before
    // the new queue renders.
    const resetSession = () => {
      setDoneCount(0);
      setAgainCount(0);
      setMoreDue(0);
      setUndoStack([]);
      setLastRated(null);
      reviewTokenRef.current = null;
    };
    setError(null);
    // Disconnected start: a cached queue means a REAL session (ratings go to
    // the outbox); sample deck only when nothing is cached.
    const startFromCache = async (): Promise<void> => {
      const cached = await loadQueue(identity);
      if (epochRef.current !== epoch) return;
      resetSession();
      if (cached.length > 0) {
        setMode('offline');
        setLiveQueue(cached);
        setPhase('front');
      } else {
        setMode('sample');
        setSampleQueue(SAMPLE_DUE_QUEUE);
        setPhase(SAMPLE_DUE_QUEUE.length > 0 ? 'front' : 'done');
      }
    };
    setSyncNotice(null);
    setNotice(null);
    try {
      if (!live) {
        await startFromCache();
        return;
      }
      // Flip to loading BEFORE the async outbox check so the tap acknowledges.
      if (!fromDone) {
        resetSession();
        setPhase('loading');
      }
      setMode('live');
      // Unsent offline ratings must land BEFORE fetching a live queue, or the
      // fetch won't reflect them and later replay applies ratings out of order.
      const outbox = await loadOutbox(identity);
      if (outbox.length > 0) {
        const res = await replayOutbox(server.client!, identity);
        if (epochRef.current !== epoch) return;
        setPendingSync(res.remaining);
        if (res.skipped > 0) {
          setSyncNotice(
            `${res.skipped} offline ${res.skipped === 1 ? 'rating' : 'ratings'} superseded on another device`,
          );
        }
        if (res.remaining > 0) {
          // Sync didn't get through — run the cached session rather than
          // fetch a queue that would conflict.
          await startFromCache();
          return;
        }
        invalidate('stats');
      }
      const fetched = await server.client!.getDueCards(20, newLimit);
      if (epochRef.current !== epoch) return;
      if (fromDone) resetSession();
      const previewAt = Date.now() / 1000;
      const cards = fetched.map((c) => ({ ...c, preview_at: previewAt }));
      setLiveQueue(cards);
      setPhase(cards.length > 0 ? 'front' : 'done');
      void saveQueue(identity, cards);
      markQueueWarmed(identity);
      setOfflineCount(cards.length);
    } catch (err) {
      if (epochRef.current !== epoch) return;
      setError(errorText(err));
      setPhase(fromDone ? 'done' : 'idle');
    } finally {
      startingRef.current = false;
      setStarting(false);
    }
  }, [live, server.client, identity]);

  // A plain start is PURE reviews (new_limit 0); new words enter only via learnMore.
  const start = useCallback(async () => {
    if (startingRef.current) return;
    startingRef.current = true;
    setStarting(true);
    await startInner(0, phase === 'done');
  }, [phase, startInner]);

  const usesLiveQueue = mode === 'live' || mode === 'offline';
  const card: ReviewCardVM | null =
    usesLiveQueue
      ? (liveQueue[0] ? dueCardToVM(liveQueue[0]) : null)
      : mode === 'sample'
        ? (sampleQueue[0] ? sampleToVM(sampleQueue[0]) : null)
        : null;
  const currentDue: DueCard | null = usesLiveQueue ? liveQueue[0] ?? null : null;
  const nextDue: DueCard | null = usesLiveQueue ? liveQueue[1] ?? null : null;
  const cardId = card?.id;

  // A never-reviewed card can't be recall-tested, so reveal it immediately.
  // `phase` is a dependency on purpose: End keeps the queue, so a restart can
  // land the SAME new card back in 'front' without `cardId` changing.
  useEffect(() => {
    if (usesLiveQueue && card?.isNew && phase === 'front') setPhase('back');
  }, [usesLiveQueue, card?.isNew, cardId, phase]);

  const remaining = usesLiveQueue ? liveQueue.length : sampleQueue.length;
  const sessionTotal = doneCount + remaining;

  const finishSession = useCallback(() => {
    setPhase('done');
    if (mode !== 'live' || !server.client) {
      setDoneReady(true);
      return;
    }
    setDoneReady(false);
    // Refresh the idle kicker stats now, in parallel, so "End study" doesn't
    // land on pre-session numbers. Doesn't gate doneReady.
    server.client.stats()
      .then((st) => setIdleStats(st))
      .catch(() => { /* kicker keeps its last values */ });
    const run = async () => {
      // One deck_stats round-trip settles the done screen; the gated count
      // matches the server's due fetch cap math.
      try {
        const s = await server.client!.deckStats();
        const left = gatedDueCount(s);
        setMoreDue(left);
        setCatchUp(s.catch_up ?? null);
        setNewPending(s.new_total ?? 0);
      } catch {
        setMoreDue(0);
      }
      setDoneReady(true);
    };
    void run();
  }, [mode, server.client]);

  // 🌱 "Learn N new words" — the ONLY way new cards enter a session. The
  // fetch's new_limit is anchored to the SERVER's new_introduced_today, so
  // each press releases exactly N regardless of reloads or earlier presses.
  // Due reviews still lead the fetched batch.
  const learnMore = useCallback(async () => {
    // Latch taken HERE, not in start(): the deckStats probe is a full
    // round-trip during which a double-tap would grant two batches.
    if (startingRef.current) return;
    startingRef.current = true;
    setStarting(true);
    const fromDone = phase === 'done';
    let introduced = 0;
    if (live && server.client) {
      try {
        const s = await server.client.deckStats();
        introduced = s.new_introduced_today ?? 0;
      } catch {
        // 0 anchor: the server still subtracts what it introduced, so this
        // degrades to "at most N minus today's intros", never an over-grant.
      }
    }
    await startInner(introduced + settings.newPerLearn, fromDone);
  }, [phase, startInner, live, server.client, settings.newPerLearn]);

  const rate = useCallback(async (r: (typeof RATINGS)[number]) => {
    setError(null);
    // Snapshot BEFORE this rating; for live, committed only after the write succeeds.
    const snapshot: UndoSnapshot = { liveQueue, sampleQueue, doneCount, againCount };
    // againRepeats: an Again'd card re-enters via reinsertAgain and stays
    // inside `remaining`, so sessionTotal holds steady — doneCount moves only
    // when a card actually leaves the queue. Off = the card just leaves.
    const repeat = r.key === 'again' && settings.againRepeats;
    // Shared tail once the rating is on the books. `token` names the write so
    // undo can match it (outbox entry offline, server review_id live).
    const commitRating = (
      lemma: string,
      remaining: number,
      token?: string,
      grad: RatingReceipt['grad'] = null,
    ) => {
      if (!repeat) setDoneCount(doneCount + 1);
      if (r.key === 'again') setAgainCount(againCount + 1);
      setUndoStack((prev) => [...prev.slice(-(UNDO_STACK_MAX - 1)), { ...snapshot, reviewId: token }]);
      setLastRated({ label: r.label, lemma, grad });
      if (remaining > 0) setPhase('front');
      else finishSession();
    };
    if (mode === 'sample') {
      const [current, ...rest] = sampleQueue;
      if (!current) return;
      const next = repeat ? reinsertAgain(rest, current) : rest;
      setSampleQueue(next);
      commitRating(current.lemma, next.length);
      return;
    }
    // Queue the rating in the outbox (same idempotency token a live write
    // would carry) and advance locally; the reconnect sync delivers it.
    const queueOffline = async (current: DueCard, token: string, epoch: number) => {
      await appendPending(identity, {
        card_id: current.id,
        rating: r.value,
        review_id: token,
        // Concurrency expectation: the chain tip as this client saw it.
        // Replay drops the entry if another device moved it.
        prev_review_id: current.last_review_id ?? '',
        queued_at: Date.now(),
      });
      // The rating is in the outbox either way; the state below belongs to
      // the session that requested it.
      if (epochRef.current !== epoch) return;
      // Lap copy: no preview (it described the pre-lapse schedule), no
      // is_new (a once-seen card can be recall-tested), and its chain tip
      // advances to THIS token so the lap's own entry chains at replay.
      const next = repeat
        ? reinsertAgain(liveQueue.slice(1), {
            ...current,
            preview: undefined,
            is_new: undefined,
            last_review_id: token,
          })
        : liveQueue.slice(1);
      setLiveQueue(next);
      void saveQueue(identity, next); // a killed app resumes mid-session
      setPendingSync((n) => n + 1);
      commitRating(current.lemma, next.length, token);
    };
    if (mode === 'offline') {
      const current = liveQueue[0];
      if (!current || submittingRef.current) return;
      // Latch: appendPending is async; a second tap queued a duplicate.
      beginSubmit();
      setPendingRating(r.key);
      try {
        await queueOffline(current, newReviewId(), epochRef.current);
      } catch {
        setError('Couldn’t save this answer on your device. Free some storage and try again.');
      } finally {
        endSubmit();
        setPendingRating(null);
      }
      return;
    }
    if (mode !== 'live' || !server.client) return;
    const current = liveQueue[0];
    if (!current || submittingRef.current) return;
    const epoch = epochRef.current;
    beginSubmit();
    setPendingRating(r.key);
    // Idempotency token, reused when retrying the SAME card+rating after a
    // failure so a timed-out-but-committed POST replays instead of applying
    // FSRS twice. A different rating is a changed mind and mints a new one.
    const prevTok = reviewTokenRef.current;
    const token =
      prevTok && prevTok.cardId === current.id && prevTok.rating === r.value
        ? prevTok.token
        : newReviewId();
    reviewTokenRef.current = { cardId: current.id, rating: r.value, token };
    try {
      const res = await server.client!.reviewCard(current.id, r.value, token);
      // Cleared on success so a lap of the same card mints a fresh token.
      reviewTokenRef.current = null;
      // The review of record committed either way; only the queue/counter
      // writes below belong to this session.
      invalidate('stats');
      if (epochRef.current !== epoch) return;
      const grad = res.promoted ? 'promoted' : res.demoted ? 'demoted' : null;
      // Re-queuing is client-side only, on top of the server's write. The lap
      // copy wears the POST-review preview, drops is_new (else it auto-reveals
      // forever), and advances its chain tip to THIS token — without that a
      // cached lap copy's next offline rating is conflict-dropped at replay.
      const next = repeat
        ? reinsertAgain(liveQueue.slice(1), {
            ...current,
            preview: res.preview,
            preview_at: Date.now() / 1000,
            is_new: undefined,
            last_review_id: token,
            system_tags: res.furigana
              ? [...(current.system_tags ?? []), 'furigana']
              : current.system_tags,
          })
        : liveQueue.slice(1);
      setLiveQueue(next);
      // Keep the offline cache in step, or a later offline session re-serves
      // reviewed cards and every rating on them is conflict-dropped at replay.
      void saveQueue(identity, next);
      commitRating(current.lemma, next.length, token, grad);
    } catch (err) {
      if (epochRef.current !== epoch) return;
      // Connection lost: the session carries on offline. Same token, so if
      // the POST did commit before the line dropped, replay returns the
      // stored result instead of rating twice.
      if (isNetworkError(err)) {
        try {
          await queueOffline(current, token, epoch);
          if (epochRef.current !== epoch) return;
          reviewTokenRef.current = null;
          setMode('offline');
          setNotice('Connection lost. Answers are saved on this device and sync when you reconnect.');
        } catch {
          if (epochRef.current === epoch) {
            setError('Couldn’t save this answer on your device. Free some storage and try again.');
          }
        }
        return;
      }
      // Failed write must not advance the queue. The same rating retries with
      // the same token, so say exactly that.
      setError(`Couldn’t save. ${errorText(err)} Tap ${r.label} to retry.`);
    } finally {
      endSubmit();
      setPendingRating(null);
    }
  }, [mode, liveQueue, sampleQueue, doneCount, againCount, server.client, settings.againRepeats, finishSession, identity]);

  // Undo the last rating; repeatable. Rolls back server-side FSRS state first
  // (the server walks its own review_log the same way); the client snapshot
  // is popped only after the server write succeeds.
  const undo = useCallback(async () => {
    const snap = undoStack[undoStack.length - 1];
    if (!snap || submittingRef.current) return;
    const epoch = epochRef.current;
    setLastRated(null);
    // Offline: undo pops the outbox entry. If it's gone, the reconnect sync
    // delivered it mid-session — a local rewind would desync the server.
    if (mode === 'offline') {
      if (!snap.reviewId) return;
      // Latch: a second tap during the pop re-entered with the stale stack.
      // Everything past the await is synchronous, so releasing early is safe.
      beginSubmit();
      const removed = await popPending(identity, snap.reviewId);
      endSubmit();
      // The cache follows the outbox even if the session died mid-pop; the
      // React state below does not.
      if (removed) void saveQueue(identity, snap.liveQueue);
      if (epochRef.current !== epoch) return;
      if (!removed) {
        setError('That rating already synced. Undo it from an online session.');
        return;
      }
      setPendingSync((n) => Math.max(0, n - 1));
    }
    if (mode === 'live' && server.client) {
      beginSubmit();
      let undone: { cardId: number; reviewId: string };
      try {
        undone = await server.client!.undoReview();
      } catch (err) {
        if (epochRef.current === epoch) setError(`Couldn’t undo. ${errorText(err)}`);
        endSubmit();
        return;
      }
      endSubmit();
      invalidate('stats');
      if (epochRef.current !== epoch) return;
      // An armed token means the last POST timed out but may have COMMITTED a
      // review the client never counted. If that's what the server just
      // undid, the client already shows the right state — restoring the
      // snapshot would rewind one review too far. Exact token match: card-id
      // matching is ambiguous when consecutive reviews hit the same card.
      const armed = reviewTokenRef.current;
      if (armed && armed.token === undone.reviewId) {
        reviewTokenRef.current = null;
        setError(null);
        setPhase('back');
        return;
      }
      // The server undoes the newest review in the whole per-user log, which
      // may be another device's: then the local rating is still on the books,
      // so leave the session untouched.
      if (undone.reviewId !== snap.reviewId) {
        setError('Undid a rating made on another device. This session is unchanged.');
        return;
      }
    }
    // The rewind wins over anything still in flight (e.g. a start racing it).
    epochRef.current += 1;
    setLiveQueue(snap.liveQueue);
    setSampleQueue(snap.sampleQueue);
    setDoneCount(snap.doneCount);
    setAgainCount(snap.againCount);
    setUndoStack((prev) => prev.slice(0, -1));
    setMoreDue(0);
    setError(null);
    setPhase('back');
  }, [undoStack, mode, server.client, identity]);

  // Permanently remove a card (live only). The lemma's word_status survives
  // server-side. Not undoable (the server's undo only covers reviews).
  const removeCard = useCallback(async (target: DueCard) => {
    if (mode !== 'live' || !server.client || submittingRef.current) return;
    const epoch = epochRef.current;
    beginSubmit();
    setError(null);
    try {
      await server.client.deleteCard(target.id);
      invalidate('vocab', 'stats');
      if (reviewTokenRef.current?.cardId === target.id) reviewTokenRef.current = null;
      if (epochRef.current !== epoch) return;
      // Filter by id, not slice(1): the queue may have advanced while the
      // confirm dialog was up (web keyboard).
      const next = liveQueue.filter((c) => c.id !== target.id);
      setLiveQueue(next);
      void saveQueue(identity, next);
      // Clear undo snapshots so deletion cannot restore the card.
      setUndoStack([]);
      setLastRated(null);
      if (next.length > 0) setPhase('front');
      else finishSession();
    } catch (err) {
      if (epochRef.current === epoch) setError(`Couldn’t remove. ${errorText(err)}`);
    } finally {
      endSubmit();
    }
  }, [mode, server.client, liveQueue, finishSession, identity]);

  // Content the learner just wrote on a card (translation, explanation,
  // mnemonic). The server has it already; this keeps the queue, its undo
  // snapshots and the resumable copy showing the same card.
  // It runs after a save's round trip, so it patches the queue as it is
  // then: a rating that landed meanwhile has moved on, and patching the
  // queue from before it would bring the rated card back.
  const patchCard = useCallback((id: number, patch: Partial<DueCard>) => {
    const apply = (queue: DueCard[]) => queue.map((c) => (c.id === id ? { ...c, ...patch } : c));
    setLiveQueue((prev) => {
      if (!prev.some((c) => c.id === id)) return prev;
      const next = apply(prev);
      void saveQueue(identity, next);
      return next;
    });
    setUndoStack((prev) => prev.map((snap) => ({ ...snap, liveQueue: apply(snap.liveQueue) })));
  }, [identity]);

  const reveal = useCallback(() => setPhase('back'), []);
  // Seed the idle count from the completed session.
  const endSession = useCallback(() => {
    epochRef.current += 1;
    if (mode === 'live' || mode === 'offline') {
      setDueCount(phase === 'done' ? moreDue : liveQueue.length);
    }
    setPhase('idle');
  }, [mode, phase, moreDue, liveQueue.length]);

  return {
    live,
    mode,
    phase,
    card,
    cardId,
    currentDue,
    nextDue,
    sessionTotal,
    dueCount,
    doneCount,
    againCount,
    error,
    moreDue,
    catchUp,
    newPending,
    offlineCount,
    pendingSync,
    syncNotice,
    notice,
    doneReady,
    starting,
    submitting,
    pendingRating,
    canUndo: undoStack.length > 0,
    undoDepth: undoStack.length,
    lastRated,
    idleStats,
    start,
    rate,
    undo,
    removeCard,
    patchCard,
    learnMore,
    reveal,
    endSession,
  };
}
