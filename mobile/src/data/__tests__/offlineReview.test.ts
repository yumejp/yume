jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import AsyncStorage from '@react-native-async-storage/async-storage';
import type { YumeClient } from '../../api/client';
import { ApiError } from '../../api/client';
import type { DueCard } from '../../api/types';
import {
  appendPending,
  loadOutbox,
  loadQueue,
  popPending,
  replayOutbox,
  saveQueue,
  type PendingReview,
} from '../offlineReview';

const ID = 'http://pi|owner';

function entry(n: number, prev = ''): PendingReview {
  return { card_id: n, rating: 3, review_id: `tok-${n}`, prev_review_id: prev, queued_at: n };
}

function okResult(cardId: number, conflict = false) {
  return { card_id: cardId, new_due: '', new_state: 2, conflict: conflict || undefined };
}

beforeEach(async () => {
  await AsyncStorage.clear();
});

describe('offline queue cache', () => {
  it('round-trips the cached due queue per identity', async () => {
    const cards = [{ id: 1, lemma: '一', card_type: 'vocab' }] as unknown as DueCard[];
    await saveQueue(ID, cards);
    expect(await loadQueue(ID)).toEqual(cards);
    // Another account's cache is untouched (card ids overlap across accounts).
    expect(await loadQueue('http://pi|test')).toEqual([]);
  });
});

describe('outbox', () => {
  it('appends in order and pops only a matching newest entry', async () => {
    await appendPending(ID, entry(1));
    await appendPending(ID, entry(2));
    expect((await loadOutbox(ID)).map((e) => e.card_id)).toEqual([1, 2]);

    // Popping with the WRONG id (entry already synced / not newest) refuses.
    expect(await popPending(ID, 'tok-1')).toBe(false);
    expect((await loadOutbox(ID)).length).toBe(2);

    expect(await popPending(ID, 'tok-2')).toBe(true);
    expect((await loadOutbox(ID)).map((e) => e.card_id)).toEqual([1]);
  });
});

describe('replayOutbox', () => {
  function mockClient(impl: jest.Mock): YumeClient {
    return { reviewCard: impl } as unknown as YumeClient;
  }

  it('replays in order with idempotency tokens + chain expectations, then drains', async () => {
    await appendPending(ID, entry(1));
    await appendPending(ID, entry(2, 'tok-1'));
    const review = jest.fn().mockImplementation((id: number) => Promise.resolve(okResult(id)));
    const res = await replayOutbox(mockClient(review), ID);
    expect(res).toEqual({ sent: 2, skipped: 0, remaining: 0 });
    expect(review.mock.calls).toEqual([
      [1, 3, 'tok-1', '', 1],
      [2, 3, 'tok-2', 'tok-1', 2],
    ]);
    expect(await loadOutbox(ID)).toEqual([]);
  });

  it('drops a conflicted entry (reviewed on another device) and continues', async () => {
    await appendPending(ID, entry(1));
    await appendPending(ID, entry(2));
    const review = jest
      .fn()
      .mockResolvedValueOnce(okResult(1, true)) // superseded by desktop
      .mockResolvedValueOnce(okResult(2));
    const res = await replayOutbox(mockClient(review), ID);
    expect(res).toEqual({ sent: 1, skipped: 1, remaining: 0 });
    expect(await loadOutbox(ID)).toEqual([]);
  });

  it('resends without the timestamp when reviewed_at is rejected — the rating survives', async () => {
    await appendPending(ID, entry(1));
    const review = jest
      .fn()
      .mockRejectedValueOnce(new ApiError('reviewed_at is outside the accepted range', 400))
      .mockResolvedValueOnce(okResult(1));
    const res = await replayOutbox(mockClient(review), ID);
    expect(res).toEqual({ sent: 1, skipped: 0, remaining: 0 });
    // First attempt carried queued_at; the retry let the server stamp the time.
    expect(review.mock.calls).toEqual([
      [1, 3, 'tok-1', '', 1],
      [1, 3, 'tok-1', ''],
    ]);
    expect(await loadOutbox(ID)).toEqual([]);
  });

  it('drops a 4xx entry (e.g. card deleted) instead of damming the queue', async () => {
    await appendPending(ID, entry(1));
    await appendPending(ID, entry(2));
    const review = jest
      .fn()
      .mockRejectedValueOnce(new ApiError('card 1 not found', 400))
      .mockResolvedValueOnce(okResult(2));
    const res = await replayOutbox(mockClient(review), ID);
    expect(res).toEqual({ sent: 1, skipped: 1, remaining: 0 });
    expect(await loadOutbox(ID)).toEqual([]);
  });

  it('stops on a network failure and keeps the remainder queued', async () => {
    await appendPending(ID, entry(1));
    await appendPending(ID, entry(2));
    const review = jest
      .fn()
      .mockResolvedValueOnce(okResult(1))
      .mockRejectedValueOnce(new Error('network down'));
    const res = await replayOutbox(mockClient(review), ID);
    expect(res).toEqual({ sent: 1, skipped: 0, remaining: 1 });
    expect((await loadOutbox(ID)).map((e) => e.card_id)).toEqual([2]);
  });

  it('single-flights concurrent replays', async () => {
    await appendPending(ID, entry(1));
    let release!: (v: unknown) => void;
    const review = jest.fn().mockReturnValue(new Promise((r) => { release = r; }));
    const client = mockClient(review);
    const p1 = replayOutbox(client, ID);
    const p2 = replayOutbox(client, ID);
    release(okResult(1));
    const [r1, r2] = await Promise.all([p1, p2]);
    expect(review).toHaveBeenCalledTimes(1);
    expect(r1).toEqual(r2);
  });
});
