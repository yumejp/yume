jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, renderHook, waitFor } from '@testing-library/react-native';
import type { DueCard } from '../../api/types';

// The hook reads the server/settings contexts directly — mock them at module
// level so the test drives a fake client with no providers.
const mockClient = {
  deckStats: jest.fn(),
  stats: jest.fn(),
  getDueCards: jest.fn(),
  reviewCard: jest.fn(),
  undoReview: jest.fn(),
  deleteCard: jest.fn(),
};
const mockServer: { status: string; client: typeof mockClient | null; identity: string } = {
  status: 'online',
  client: mockClient,
  identity: 'test-0',
};

jest.mock('../../server/ServerContext', () => ({
  useServer: () => mockServer,
}));
// Mutable so individual tests can flip the review-flow toggles; reset in
// beforeEach to the app defaults.
const mockSettings = { newPerLearn: 5, againRepeats: true };
jest.mock('../../settings/SettingsContext', () => ({
  useSettings: () => ({ settings: mockSettings }),
}));

// eslint-disable-next-line import/first
import { RATINGS, reinsertAgain, useReviewSession } from '../useReviewSession';
// eslint-disable-next-line import/first
import { SAMPLE_DUE_QUEUE } from '../../data/sample';
// eslint-disable-next-line import/first
import { invalidate } from '../../data/queryStore';
// eslint-disable-next-line import/first
import {
  appendPending,
  loadOutbox,
  loadQueue,
  markQueueWarmed,
  replayOutbox,
  saveQueue,
} from '../../data/offlineReview';

const GOOD = RATINGS[2];
const AGAIN = RATINGS[0];

function dueCard(id: number, lemma: string): DueCard {
  return {
    id,
    lemma,
    kanji: [],
    readings: [],
    glosses: [['gloss']],
    card_type: 'vocab',
  } as unknown as DueCard;
}

// Simulated server-side review log: the default reviewCard mock pushes an
// entry, the default undoReview mock pops the newest — matching the real
// contract, where undo reverts the newest review in the whole log and
// reports its {card_id, undone_review_id} so the client can tell whose
// review it just reverted.
const serverLog: { cardId: number; reviewId: string }[] = [];

let testSeq = 0;
beforeEach(async () => {
  jest.clearAllMocks();
  await AsyncStorage.clear();
  mockServer.status = 'online';
  mockServer.client = mockClient;
  // Fresh identity per test (offline cache/outbox storage is per-identity),
  // pre-marked warmed so the idle cache prefetch doesn't race the mocks the
  // pre-existing tests arm with mockReturnValueOnce.
  mockServer.identity = `test-${++testSeq}`;
  markQueueWarmed(mockServer.identity);
  mockSettings.newPerLearn = 5;
  mockSettings.againRepeats = true;
  mockClient.deckStats.mockResolvedValue({ total: 2, due: 2, new_total: 0 });
  mockClient.stats.mockRejectedValue(new Error('no stats in test'));
  mockClient.getDueCards.mockResolvedValue([dueCard(1, '一'), dueCard(2, '二')]);
  serverLog.length = 0;
  mockClient.reviewCard.mockImplementation(async (cardId: number, _r: number, reviewId?: string) => {
    serverLog.push({ cardId, reviewId: reviewId ?? '' });
    return { card_id: cardId, new_due: '', new_state: 2 };
  });
  mockClient.undoReview.mockImplementation(async () => {
    const last = serverLog.pop();
    if (!last) throw new Error('nothing to undo');
    return last;
  });
  mockClient.deleteCard.mockResolvedValue(true);
});

async function startSession() {
  const utils = renderHook(() => useReviewSession());
  await act(async () => {
    await utils.result.current.start();
  });
  expect(utils.result.current.phase).toBe('front');
  expect(utils.result.current.card?.id).toBe(1);
  return utils;
}

describe('useReviewSession', () => {
  it('advances the queue on a successful rating', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    expect(result.current.doneCount).toBe(1);
    expect(result.current.canUndo).toBe(true);
    expect(mockClient.reviewCard).toHaveBeenCalledWith(1, GOOD.value, expect.any(String));
  });

  it('the rating receipt names the rated word and its graduation, not the next card', async () => {
    const { result } = await startSession();
    mockClient.reviewCard.mockResolvedValueOnce({ card_id: 1, new_due: '', new_state: 2, promoted: true });
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.lemma).toBe('二');
    expect(result.current.lastRated).toEqual({ label: GOOD.label, lemma: '一', grad: 'promoted' });
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.lastRated).toEqual({ label: GOOD.label, lemma: '二', grad: null });
  });

  it('NEVER advances the queue on a failed write (the core invariant)', async () => {
    mockClient.reviewCard.mockRejectedValueOnce(new Error('network down'));
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(1); // stayed put
    expect(result.current.doneCount).toBe(0);
    expect(result.current.canUndo).toBe(false); // no snapshot armed
    expect(result.current.error).toContain('network down');
  });

  it('keeps a live retry token when both the network and device storage fail', async () => {
    const { result } = await startSession();
    mockClient.reviewCard.mockRejectedValueOnce(new TypeError('Network request failed'));
    (AsyncStorage.setItem as jest.Mock).mockRejectedValueOnce(new Error('storage full'));
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.card?.id).toBe(1);
    expect(result.current.doneCount).toBe(0);
    expect(result.current.mode).toBe('live');
    expect(result.current.error).toContain('Free some storage');
    const token = mockClient.reviewCard.mock.calls[0][2];
    await act(async () => { await result.current.rate(GOOD); });
    expect(mockClient.reviewCard.mock.calls[1][2]).toBe(token);
    expect(result.current.card?.id).toBe(2);
  });

  it('a live rating that cannot reach the server is queued offline and the session continues', async () => {
    const { result } = await startSession();
    mockClient.reviewCard.mockRejectedValueOnce(new TypeError('Network request failed'));
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.lemma).toBe('二');
    expect(result.current.mode).toBe('offline');
    expect(result.current.doneCount).toBe(1);
    expect(result.current.pendingSync).toBe(1);
    expect(result.current.error).toBeNull();
    expect(result.current.notice).toMatch(/Connection lost/);
    const outbox = await loadOutbox(mockServer.identity);
    expect(outbox).toHaveLength(1);
    expect(outbox[0].card_id).toBe(1);
    // Same token the failed POST carried, so a committed-but-timed-out write replays, not doubles.
    expect(outbox[0].review_id).toBe(mockClient.reviewCard.mock.calls[0][2]);
    // The next rating stays offline without another round-trip.
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(mockClient.reviewCard).toHaveBeenCalledTimes(1);
    expect(await loadOutbox(mockServer.identity)).toHaveLength(2);
  });

  describe('after a live session drops offline', () => {
    const dropOffline = async () => {
      const utils = await startSession();
      mockClient.reviewCard.mockRejectedValueOnce(new TypeError('Network request failed'));
      await act(async () => {
        await utils.result.current.rate(GOOD);
      });
      expect(utils.result.current.mode).toBe('offline');
      return utils;
    };

    it('undo pops the queued rating and a re-rate queues a fresh one off the same chain tip', async () => {
      const { result } = await dropOffline();
      const [first] = await loadOutbox(mockServer.identity);
      await act(async () => {
        await result.current.undo();
      });
      expect(result.current.card?.id).toBe(1);
      expect(result.current.doneCount).toBe(0);
      expect(result.current.pendingSync).toBe(0);
      expect(await loadOutbox(mockServer.identity)).toEqual([]);
      await act(async () => {
        await result.current.rate(AGAIN);
      });
      const [again] = await loadOutbox(mockServer.identity);
      expect(again.rating).toBe(AGAIN.value);
      expect(again.review_id).not.toBe(first.review_id);
      expect(again.prev_review_id).toBe(first.prev_review_id);
      expect(mockClient.reviewCard).toHaveBeenCalledTimes(1); // still offline, no POST
    });

    it('cannot undo past the drop into a rating the server already has', async () => {
      const { result } = await startSession();
      await act(async () => {
        await result.current.rate(GOOD); // live, committed
      });
      mockClient.reviewCard.mockRejectedValueOnce(new TypeError('Network request failed'));
      await act(async () => {
        await result.current.rate(GOOD); // drops offline
      });
      await act(async () => {
        await result.current.undo(); // pops the offline entry
      });
      expect(result.current.card?.id).toBe(2);
      await act(async () => {
        await result.current.undo(); // targets the live rating
      });
      expect(result.current.card?.id).toBe(2); // unchanged
      expect(result.current.error).toMatch(/already synced/);
      expect(mockClient.undoReview).not.toHaveBeenCalled();
      expect(serverLog).toHaveLength(1);
    });

    it('undo after the reconnect sync delivered the rating does not rewind', async () => {
      const { result } = await dropOffline();
      // The reconnect sync (OfflineSync) drains the outbox behind the session.
      await act(async () => {
        await replayOutbox(mockClient as never, mockServer.identity);
      });
      expect(serverLog).toHaveLength(1);
      await act(async () => {
        await result.current.undo();
      });
      expect(result.current.card?.id).toBe(2);
      expect(result.current.doneCount).toBe(1);
      expect(result.current.error).toMatch(/already synced/);
    });

    it('an Again lap after the drop chains off the queued rating', async () => {
      const { result } = await dropOffline();
      await act(async () => {
        await result.current.rate(AGAIN); // card 2, queued
      });
      // Walk the queue until the lap copy of card 2 comes back around.
      while (result.current.card?.id !== 2) {
        await act(async () => {
          await result.current.rate(GOOD);
        });
      }
      await act(async () => {
        await result.current.rate(GOOD); // lap of card 2
      });
      const outbox = await loadOutbox(mockServer.identity);
      const twos = outbox.filter((e) => e.card_id === 2);
      expect(twos).toHaveLength(2);
      expect(twos[1].prev_review_id).toBe(twos[0].review_id);
    });
  });

  it('reuses the same idempotency token when retrying the same card+rating', async () => {
    mockClient.reviewCard.mockRejectedValueOnce(new Error('timeout'));
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    await act(async () => {
      await result.current.rate(GOOD);
    });
    const [first, second] = mockClient.reviewCard.mock.calls;
    expect(first[2]).toBe(second[2]); // same token → server replays, not re-applies
    expect(result.current.card?.id).toBe(2); // retry succeeded and advanced
  });

  it('issues a fresh token for a different rating (changed mind)', async () => {
    mockClient.reviewCard.mockRejectedValueOnce(new Error('timeout'));
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    await act(async () => {
      await result.current.rate(AGAIN);
    });
    const [first, second] = mockClient.reviewCard.mock.calls;
    expect(first[2]).not.toBe(second[2]);
  });

  it('undo restores the exact pre-rating snapshot after a server rollback', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    await act(async () => {
      await result.current.undo();
    });
    expect(mockClient.undoReview).toHaveBeenCalled();
    expect(result.current.card?.id).toBe(1);
    expect(result.current.doneCount).toBe(0);
    expect(result.current.phase).toBe('back'); // card returns face-up
    expect(result.current.canUndo).toBe(false); // single-step
  });

  it('undoes multiple steps, one server rollback per step', async () => {
    mockSettings.againRepeats = false; // Again must END the session here
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    await act(async () => {
      await result.current.rate(AGAIN);
    });
    expect(result.current.phase).toBe('done'); // both cards rated
    expect(result.current.undoDepth).toBe(2);

    await act(async () => {
      await result.current.undo();
    });
    expect(result.current.card?.id).toBe(2);
    expect(result.current.doneCount).toBe(1);
    expect(result.current.undoDepth).toBe(1);

    await act(async () => {
      await result.current.undo();
    });
    expect(result.current.card?.id).toBe(1);
    expect(result.current.doneCount).toBe(0);
    expect(result.current.canUndo).toBe(false);
    expect(mockClient.undoReview).toHaveBeenCalledTimes(2);
  });

  it('drops a start that resolves after the client identity changed', async () => {
    // Cross-account safety: a getDueCards in flight when the account switches
    // must NOT load the old user's cards under the new client (card ids
    // overlap across accounts — rating them would hit the wrong deck).
    let release!: (cards: DueCard[]) => void;
    mockClient.getDueCards.mockReturnValueOnce(
      new Promise<DueCard[]>((res) => { release = res; }),
    );
    const utils = renderHook(() => useReviewSession());
    let startP!: Promise<void>;
    act(() => { startP = utils.result.current.start(); });
    expect(utils.result.current.phase).toBe('loading');

    // Account switch: new client identity → reset effect runs.
    mockServer.client = { ...mockClient } as typeof mockClient;
    utils.rerender(undefined);
    expect(utils.result.current.phase).toBe('idle');

    await act(async () => {
      release([dueCard(1, '一'), dueCard(2, '二')]);
      await startP;
    });
    expect(utils.result.current.phase).toBe('idle'); // dead session stays dead
    expect(utils.result.current.card).toBeNull();
  });

  it('a rating that resolves after End cannot resurrect the ended session', async () => {
    const { result } = await startSession();
    await act(async () => { await result.current.reveal(); });
    let release!: (r: unknown) => void;
    mockClient.reviewCard.mockReturnValueOnce(
      new Promise((res) => { release = res; }),
    );
    let rateP!: Promise<void>;
    act(() => { rateP = result.current.rate(GOOD); });
    // The POST hangs; the user gives up and ends the session.
    act(() => { result.current.endSession(); });
    expect(result.current.phase).toBe('idle');

    await act(async () => {
      release({ card_id: 1, new_due: '', new_state: 2 });
      await rateP;
    });
    // The late success must not advance the dead queue or flip the phase.
    expect(result.current.phase).toBe('idle');
    expect(result.current.doneCount).toBe(0);
    expect(result.current.canUndo).toBe(false);
  });

  it('learnMore is single-flight: a double-tap grants one batch, one fetch', async () => {
    mockClient.deckStats.mockResolvedValue({ total: 2, due: 0, new_total: 6, new_introduced_today: 0 });
    const utils = renderHook(() => useReviewSession());
    await act(async () => {
      const p1 = utils.result.current.learnMore();
      const p2 = utils.result.current.learnMore(); // double-tap
      await Promise.all([p1, p2]);
    });
    expect(mockClient.getDueCards).toHaveBeenCalledTimes(1);
    // ONE grant of newPerLearn (5) anchored to introduced-today (0) — not two.
    expect(mockClient.getDueCards).toHaveBeenCalledWith(20, 5);
  });

  it('undo reconciles a committed-but-timed-out rating instead of over-rewinding', async () => {
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    // Card 1 rates fine.
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.card?.id).toBe(2);
    // Card 2's POST times out — but the server actually committed it: the
    // rejected call still recorded the client's token, so log the hidden
    // commit under it, exactly as the real server would have.
    mockClient.reviewCard.mockRejectedValueOnce(new Error('timeout'));
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.card?.id).toBe(2); // failed write never advances
    const [hiddenCardId, , hiddenToken] = mockClient.reviewCard.mock.calls.at(-1)!;
    serverLog.push({ cardId: hiddenCardId, reviewId: hiddenToken });
    // Undo: the server undoes its NEWEST review — the hidden card-2 one.
    await act(async () => { await result.current.undo(); });
    // Client must NOT restore the pre-card-1 snapshot: it's already showing
    // the correct state (card 2, unrated on the server again).
    expect(result.current.card?.id).toBe(2);
    expect(result.current.undoDepth).toBe(1); // card 1's snapshot still there
    // A second undo now rolls back card 1 normally.
    await act(async () => { await result.current.undo(); });
    expect(result.current.card?.id).toBe(1);
    expect(result.current.undoDepth).toBe(0);
  });

  it('undo of another device\'s newer review leaves this session untouched', async () => {
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.card?.id).toBe(2);
    // Another client reviews a card AFTER this session's rating — its
    // review is now the newest in the log, so a server undo reverts IT.
    serverLog.push({ cardId: 99, reviewId: 'ext-token-1' });
    await act(async () => { await result.current.undo(); });
    // The local rewind must not run: this session's rating is still on the
    // books server-side, and restoring the snapshot would desync the logs.
    expect(result.current.card?.id).toBe(2);
    expect(result.current.doneCount).toBe(1);
    expect(result.current.undoDepth).toBe(1);
    expect(result.current.error).toContain('another device');
    // With the foreign review gone, undo now reverts this session's rating.
    await act(async () => { await result.current.undo(); });
    expect(result.current.card?.id).toBe(1);
    expect(result.current.doneCount).toBe(0);
  });

  it('a live rating keeps the offline queue cache in step', async () => {
    // Regression: the cache was only written at session start, so a later
    // offline session re-served already-reviewed cards and every rating made
    // on them was conflict-dropped at replay.
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    await act(async () => { await result.current.rate(GOOD); });
    expect(await loadQueue(mockServer.identity)).toHaveLength(1);
    await act(async () => { await result.current.rate(GOOD); });
    expect(await loadQueue(mockServer.identity)).toHaveLength(0);
  });

  it('keeps the done screen up (no idle flash) while "Keep going" fetches', async () => {
    // Regression: starting the next batch from the done screen used to flip
    // phase to 'loading', which rendered the idle screen (with a stale due
    // count) until the fetch landed — the "layout shift when I finish a
    // session and keep going".
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    await act(async () => { await result.current.rate(GOOD); });
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.phase).toBe('done');
    expect(result.current.doneCount).toBe(2);

    // Hold the next batch's fetch open and start from the done screen.
    let release!: (cards: DueCard[]) => void;
    mockClient.getDueCards.mockReturnValueOnce(
      new Promise<DueCard[]>((res) => { release = res; }),
    );
    let startP!: Promise<void>;
    act(() => { startP = result.current.start(); });
    // Mid-fetch: still the done screen, counts untouched, spinner flag on.
    expect(result.current.phase).toBe('done');
    expect(result.current.doneCount).toBe(2);
    expect(result.current.starting).toBe(true);

    await act(async () => {
      release([dueCard(3, '三')]);
      await startP;
    });
    expect(result.current.phase).toBe('front');
    expect(result.current.card?.id).toBe(3);
    expect(result.current.doneCount).toBe(0); // counters reset with the new queue
    expect(result.current.starting).toBe(false);
  });

  it('refreshes the done screen\'s "left" count when stats are invalidated', async () => {
    // Regression: the tab badge follows every 'stats' bump (heartbeat, other
    // devices) but the done screen kept the count from finishSession.
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    await act(async () => { await result.current.rate(GOOD); });
    await act(async () => { await result.current.rate(GOOD); });
    expect(result.current.phase).toBe('done');
    await waitFor(() => expect(result.current.doneReady).toBe(true));
    expect(result.current.moreDue).toBe(2);

    mockClient.deckStats.mockResolvedValue({ total: 9, due: 7, new_total: 0 });
    act(() => { invalidate('stats'); });
    await waitFor(() => expect(result.current.moreDue).toBe(7));
    expect(result.current.phase).toBe('done');
  });

  it('does not restore the client snapshot when the server undo fails', async () => {
    mockClient.undoReview.mockRejectedValueOnce(new Error('nothing to undo'));
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    await act(async () => {
      await result.current.undo();
    });
    expect(result.current.card?.id).toBe(2); // queue untouched
    expect(result.current.error).toContain('nothing to undo');
  });

  it('removeCard deletes by id, not queue position', async () => {
    const { result } = await startSession();
    const first = dueCard(1, '一');
    // Simulate the web race: the queue advanced past card 1 while the confirm
    // dialog was up, then the user confirmed removing card 1.
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    await act(async () => {
      await result.current.removeCard(first);
    });
    expect(mockClient.deleteCard).toHaveBeenCalledWith(1);
    expect(result.current.card?.id).toBe(2); // card 2 must NOT be dropped
  });

  it('patchCard updates the card in the queue, its undo snapshots and the saved copy', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    act(() => {
      result.current.patchCard(2, { translation: 'Two.', explanation: 'Mine.' });
    });
    expect(result.current.card?.sentenceTranslation).toBe('Two.');
    expect(result.current.card?.explanation).toBe('Mine.');
    await waitFor(async () => {
      expect((await loadQueue(mockServer.identity)).find((c) => c.id === 2)?.translation).toBe('Two.');
    });

    // Undo restores the pre-rating queue with the learner's edit kept.
    await act(async () => {
      await result.current.undo();
    });
    expect(result.current.card?.id).toBe(1);
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.explanation).toBe('Mine.');
  });

  it('patchCard from before a rating does not bring the rated card back', async () => {
    // A save started on card 1 can resolve after card 1's rating landed.
    const { result } = await startSession();
    const patchFromBefore = result.current.patchCard;
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    act(() => {
      patchFromBefore(1, { explanation: 'Mine.' });
    });
    expect(result.current.card?.id).toBe(2);
    await waitFor(async () => {
      expect((await loadQueue(mockServer.identity)).map((c) => c.id)).not.toContain(1);
    });
  });

  it('keeps an in-progress session across a transient offline flap', async () => {
    // Regression: a foreground re-probe hitting a sleepy tailnet briefly
    // reads 'offline'; that used to dump the whole queue back to idle.
    const { result, rerender } = await startSession();
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);

    mockServer.status = 'offline';
    rerender(undefined);
    expect(result.current.phase).toBe('front');
    expect(result.current.card?.id).toBe(2); // queue intact while offline

    // Rating while offline fails → error, no advance, still recoverable.
    mockClient.reviewCard.mockRejectedValueOnce(new Error('unreachable'));
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBe(2);
    expect(result.current.error).toContain('unreachable');

    mockServer.status = 'online';
    rerender(undefined);
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card?.id).toBeUndefined(); // queue drained normally
    expect(result.current.doneCount).toBe(2);
  });

  it('resets the session when the client identity changes (account switch)', async () => {
    const { result, rerender } = await startSession();
    // New client object = new URL/account/test-user.
    mockServer.client = { ...mockClient } as typeof mockClient;
    rerender(undefined);
    await waitFor(() => expect(result.current.phase).toBe('idle'));
    expect(result.current.card).toBeNull();
  });

  // --- "again repeats this session" -----------------------------------------

  it('Again re-appends the card to the queue tail when againRepeats is on', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(AGAIN);
    });
    // The write of record happened, but the card comes back this sitting.
    expect(mockClient.reviewCard).toHaveBeenCalledWith(1, AGAIN.value, expect.any(String));
    expect(result.current.card?.id).toBe(2); // advanced to the next card…
    expect(result.current.againCount).toBe(1);
    expect(result.current.doneCount).toBe(0); // …but 1 hasn't LEFT the session
    await act(async () => {
      await result.current.rate(GOOD); // clear card 2
    });
    expect(result.current.card?.id).toBe(1); // the Again'd card laps around
    expect(result.current.phase).toBe('front');
  });

  it('a lap card wears the POST-review preview, not the stale fetched one', async () => {
    const now = Math.round(Date.now() / 1000);
    const withPreview = {
      ...dueCard(1, '一'),
      // Fetched pre-lapse: a mature card whose Hard would be ~10 days out.
      preview: { again: now + 600, hard: now + 10 * 86400, good: now + 17 * 86400, easy: now + 30 * 86400 },
    } as DueCard;
    mockClient.getDueCards.mockResolvedValue([withPreview, dueCard(2, '二')]);
    // The Again commit lapses it server-side — relearning steps are minutes.
    mockClient.reviewCard.mockResolvedValue({
      card_id: 1,
      new_due: '',
      new_state: 3,
      preview: { again: now + 300, hard: now + 600, good: now + 600, easy: now + 2 * 86400 },
    });
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(AGAIN);
    });
    await act(async () => {
      await result.current.rate(GOOD); // clear card 2
    });
    expect(result.current.card?.id).toBe(1); // the Again'd card laps around…
    expect(result.current.card?.intervals?.hard).toBe('10m'); // …relabeled, not '10d'
  });

  it('Again advances past the card when againRepeats is off', async () => {
    mockSettings.againRepeats = false;
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(AGAIN);
    });
    expect(result.current.card?.id).toBe(2);
    expect(result.current.doneCount).toBe(1); // left the session for real
    await act(async () => {
      await result.current.rate(GOOD);
    });
    expect(result.current.card).toBeNull(); // no lap — session drains
  });

  it('a re-encountered Again card gets a FRESH idempotency token', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(AGAIN); // card 1, first encounter
    });
    await act(async () => {
      await result.current.rate(GOOD); // card 2
    });
    await act(async () => {
      await result.current.rate(AGAIN); // card 1 again — a NEW review
    });
    const card1Calls = mockClient.reviewCard.mock.calls.filter((c) => c[0] === 1);
    expect(card1Calls).toHaveLength(2);
    // Same card, same rating — but the first write SUCCEEDED, so the second
    // encounter must not replay it (a reused token would make the server
    // return the first review's result instead of recording a new one).
    expect(card1Calls[0][2]).not.toBe(card1Calls[1][2]);
  });

  it('undo after an Again-repeat restores the exact pre-rating queue', async () => {
    const { result } = await startSession();
    await act(async () => {
      await result.current.rate(AGAIN); // queue [1,2] → [2,1]
    });
    expect(result.current.card?.id).toBe(2);
    await act(async () => {
      await result.current.undo();
    });
    // Snapshot semantics: the re-appended copy is gone, 1 is current again.
    expect(result.current.card?.id).toBe(1);
    expect(result.current.againCount).toBe(0);
    expect(result.current.phase).toBe('back');
  });

  // --- on-request new words ---------------------------------------------------

  it('a plain start is PURE reviews — new_limit 0 even with new cards pending', async () => {
    mockClient.deckStats.mockResolvedValue({
      total: 10, due: 8, new_total: 3, due_reviews: 5, due_new: 3,
    });
    const { result } = await startSession();
    expect(result.current.card?.id).toBe(1);
    const lastFetch = mockClient.getDueCards.mock.calls.at(-1)!;
    expect(lastFetch[1]).toBe(0); // new words only ever enter via 🌱 learnMore
  });

  it('learnMore anchors its grant to the server’s introduced-today count', async () => {
    // 4 already introduced today (an earlier press, another device): the next
    // press must still release exactly newPerLearn more — new_limit 4 + 5.
    mockClient.deckStats.mockResolvedValue({
      total: 10, due: 3, new_total: 6, due_reviews: 0, due_new: 6, new_introduced_today: 4,
    });
    const utils = renderHook(() => useReviewSession());
    await act(async () => { await utils.result.current.learnMore(); });
    expect(mockClient.getDueCards).toHaveBeenCalledWith(20, 9);
  });

  it('learnMore still grants when the stats probe fails (0 anchor, never blocked)', async () => {
    mockClient.deckStats.mockRejectedValue(new Error('tailnet asleep'));
    const utils = renderHook(() => useReviewSession());
    await act(async () => { await utils.result.current.learnMore(); });
    expect(mockClient.getDueCards).toHaveBeenCalledWith(20, 5);
  });

  // --- staleness refresh (invalidate('stats') → idle refetch) ----------------

  it("a 'stats' bump refreshes the idle screen but never refetches mid-session", async () => {
    const utils = renderHook(() => useReviewSession());
    await waitFor(() => expect(mockClient.deckStats).toHaveBeenCalled());
    const idleCalls = mockClient.deckStats.mock.calls.length;
    // Idle: a bump (app came back after a while / a write elsewhere) refetches.
    mockClient.deckStats.mockResolvedValue({ total: 9, due: 9, new_total: 0 });
    act(() => { invalidate('stats'); });
    await waitFor(() => expect(mockClient.deckStats.mock.calls.length).toBeGreaterThan(idleCalls));
    await waitFor(() => expect(utils.result.current.dueCount).toBe(9));
    // Mid-session: every rating bumps 'stats'; that must NOT cost a
    // deck_stats round-trip per card.
    await act(async () => { await utils.result.current.start(); });
    const inSession = mockClient.deckStats.mock.calls.length;
    act(() => { invalidate('stats'); });
    await act(async () => { await Promise.resolve(); });
    expect(mockClient.deckStats.mock.calls.length).toBe(inSession);
  });

  // --- offline review ---------------------------------------------------------

  describe('offline sessions', () => {
    beforeEach(async () => {
      mockServer.status = 'offline';
      await saveQueue(mockServer.identity, [
        { ...dueCard(1, '一'), last_review_id: 'srv-1' },
        dueCard(2, '二'),
      ]);
    });

    it('keeps the current card when its offline answer cannot be persisted', async () => {
      const { result } = renderHook(() => useReviewSession());
      await act(async () => { await result.current.start(); });
      (AsyncStorage.setItem as jest.Mock).mockRejectedValueOnce(new Error('storage full'));
      await act(async () => { await result.current.rate(GOOD); });
      expect(result.current.card?.id).toBe(1);
      expect(result.current.doneCount).toBe(0);
      expect(result.current.pendingSync).toBe(0);
      expect(result.current.error).toContain('Free some storage');
      expect(await loadOutbox(mockServer.identity)).toEqual([]);
    });

    it('starts a REAL session from the cached queue instead of the sample deck', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      expect(utils.result.current.mode).toBe('offline');
      expect(utils.result.current.card?.id).toBe(1);
    });

    it('rating queues to the outbox (with the chain expectation), never POSTs', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(GOOD); });
      expect(utils.result.current.card?.id).toBe(2);
      expect(utils.result.current.doneCount).toBe(1);
      expect(mockClient.reviewCard).not.toHaveBeenCalled();
      const outbox = await loadOutbox(mockServer.identity);
      expect(outbox).toHaveLength(1);
      expect(outbox[0]).toMatchObject({ card_id: 1, rating: GOOD.value, prev_review_id: 'srv-1' });
      // The shrinking queue is persisted, so a killed app resumes here.
      expect((await loadQueue(mockServer.identity)).map((c) => c.id)).toEqual([2]);
    });

    it('an offline Again-lap chains its second rating off the first', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(AGAIN); }); // 1 laps
      await act(async () => { await utils.result.current.rate(GOOD); }); // clear 2
      expect(utils.result.current.card?.id).toBe(1); // the lap
      await act(async () => { await utils.result.current.rate(GOOD); });
      const outbox = await loadOutbox(mockServer.identity);
      expect(outbox).toHaveLength(3);
      const card1 = outbox.filter((e) => e.card_id === 1);
      // Second card-1 entry expects the FIRST card-1 entry's token — the
      // chain the server verifies at replay.
      expect(card1[1].prev_review_id).toBe(card1[0].review_id);
    });

    it('latches out a rating double-tap: one outbox entry, one undoable snapshot', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      // Second tap fires while the first's outbox write is still in flight —
      // it must be swallowed, not queue a duplicate entry.
      await act(async () => {
        const first = utils.result.current.rate(GOOD);
        const second = utils.result.current.rate(GOOD);
        await Promise.all([first, second]);
      });
      expect(utils.result.current.card?.id).toBe(2);
      expect(utils.result.current.doneCount).toBe(1);
      expect(utils.result.current.pendingSync).toBe(1);
      expect(utils.result.current.undoDepth).toBe(1);
      expect(await loadOutbox(mockServer.identity)).toHaveLength(1);
      // And the undo REALLY undoes — no orphaned first entry left to sync.
      await act(async () => { await utils.result.current.undo(); });
      expect(utils.result.current.card?.id).toBe(1);
      expect(await loadOutbox(mockServer.identity)).toHaveLength(0);
    });

    it('offline undo pops the outbox entry and restores the snapshot', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(GOOD); });
      expect(utils.result.current.card?.id).toBe(2);
      await act(async () => { await utils.result.current.undo(); });
      expect(utils.result.current.card?.id).toBe(1);
      expect(utils.result.current.doneCount).toBe(0);
      expect(await loadOutbox(mockServer.identity)).toHaveLength(0);
      expect(mockClient.undoReview).not.toHaveBeenCalled();
    });

    it('a live start drains the outbox FIRST, in order, before fetching', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(GOOD); });
      await act(async () => { await utils.result.current.rate(GOOD); });
      expect(utils.result.current.phase).toBe('done');

      // Connection returns; a new start must replay both ratings before the
      // due fetch (order: reviewCard, reviewCard, getDueCards).
      mockServer.status = 'online';
      mockClient.reviewCard.mockResolvedValue({ card_id: 1, new_due: '', new_state: 2 });
      utils.rerender(undefined);
      await act(async () => { await utils.result.current.start(); });
      expect(mockClient.reviewCard).toHaveBeenCalledTimes(2);
      expect(mockClient.reviewCard.mock.calls[0][0]).toBe(1);
      expect(mockClient.reviewCard.mock.calls[0][3]).toBe('srv-1'); // expectation rides along
      expect(mockClient.reviewCard.mock.calls[1][0]).toBe(2);
      expect(utils.result.current.mode).toBe('live');
      expect(await loadOutbox(mockServer.identity)).toHaveLength(0);
      const reviewOrder = mockClient.reviewCard.mock.invocationCallOrder;
      const fetchOrder = mockClient.getDueCards.mock.invocationCallOrder.at(-1)!;
      expect(Math.max(...reviewOrder)).toBeLessThan(fetchOrder);
    });

    it('surfaces conflict-skipped ratings after a drain', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(GOOD); });

      mockServer.status = 'online';
      // Desktop reviewed card 1 meanwhile — the server refuses the stale entry.
      mockClient.reviewCard.mockResolvedValue({ card_id: 1, new_due: '', new_state: 2, conflict: true });
      utils.rerender(undefined);
      await act(async () => { await utils.result.current.start(); });
      expect(utils.result.current.syncNotice).toContain('superseded');
      expect(await loadOutbox(mockServer.identity)).toHaveLength(0);
    });

    it('falls back to the offline session when the drain fails mid-"online"', async () => {
      const utils = renderHook(() => useReviewSession());
      await act(async () => { await utils.result.current.start(); });
      await act(async () => { await utils.result.current.rate(GOOD); });
      await act(async () => { utils.result.current.endSession(); });

      mockServer.status = 'online'; // optimistic — but the POST still fails
      mockClient.reviewCard.mockRejectedValue(new Error('tailnet asleep'));
      utils.rerender(undefined);
      await act(async () => { await utils.result.current.start(); });
      expect(utils.result.current.mode).toBe('offline'); // cached session, no live fetch
      expect(utils.result.current.card?.id).toBe(2); // resumes the persisted remainder
      expect((await loadOutbox(mockServer.identity))).toHaveLength(1); // rating kept for later
    });
  });

  // --- randomized Again re-entry ---------------------------------------------

  describe('reinsertAgain', () => {
    afterEach(() => jest.spyOn(Math, 'random').mockRestore());

    it('inserts at least MIN_AGAIN_GAP cards ahead on long queues', () => {
      const rest = ['a', 'b', 'c', 'd', 'e', 'f'];
      jest.spyOn(Math, 'random').mockReturnValue(0);
      expect(reinsertAgain(rest, 'X')).toEqual(['a', 'b', 'c', 'X', 'd', 'e', 'f']);
      jest.spyOn(Math, 'random').mockReturnValue(0.999);
      expect(reinsertAgain(rest, 'X')).toEqual(['a', 'b', 'c', 'd', 'e', 'f', 'X']);
    });

    it('appends on queues shorter than the gap (nothing else left to show)', () => {
      jest.spyOn(Math, 'random').mockReturnValue(0);
      expect(reinsertAgain([], 'X')).toEqual(['X']);
      expect(reinsertAgain(['a'], 'X')).toEqual(['a', 'X']);
      expect(reinsertAgain(['a', 'b'], 'X')).toEqual(['a', 'b', 'X']);
    });
  });
});
