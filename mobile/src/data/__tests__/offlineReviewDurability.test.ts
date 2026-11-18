jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
import AsyncStorage from '@react-native-async-storage/async-storage';
import { ApiError, type YumeClient } from '../../api/client';
import { appendPending, loadOutbox, replayOutbox } from '../offlineReview';

const identity = 'review-audit|owner';
const entry = { card_id: 1, rating: 3 as const, review_id: 'audit-1', prev_review_id: '', queued_at: Date.now() };
beforeEach(async () => { await AsyncStorage.clear(); });

test.each([401, 403, 408, 429, 500])('retains offline ratings after HTTP %i', async (status) => {
  await appendPending(identity, entry);
  const client = { reviewCard: jest.fn().mockRejectedValue(new ApiError('retry later', status)) } as unknown as YumeClient;
  await replayOutbox(client, identity);
  expect(await loadOutbox(identity)).toEqual([entry]);
});

test('a failed durable write must reject the rating', async () => {
  (AsyncStorage.setItem as jest.Mock).mockRejectedValueOnce(new Error('storage full'));
  await expect(appendPending(identity, entry)).rejects.toThrow('storage full');
});

test('network failure on timestamp fallback must retain the rating', async () => {
  await appendPending(identity, entry);
  const client = { reviewCard: jest.fn()
    .mockRejectedValueOnce(new ApiError('reviewed_at outside range', 400))
    .mockRejectedValueOnce(new TypeError('network unavailable')) } as unknown as YumeClient;
  await replayOutbox(client, identity);
  expect(await loadOutbox(identity)).toEqual([entry]);
});
