import { invalidate, invalidateIfStale, useInvalidationVersion } from '../queryStore';
import { renderHook, act } from '@testing-library/react-native';

describe('invalidateIfStale', () => {
  it('bumps only keys untouched for longer than maxAge', () => {
    const { result } = renderHook(() => useInvalidationVersion('stats'));
    const before = result.current;

    // Freshly bumped → a stale check is a no-op.
    act(() => { invalidate('stats'); });
    const bumped = result.current;
    expect(bumped).toBe(before + 1);
    act(() => { invalidateIfStale(60_000, 'stats'); });
    expect(result.current).toBe(bumped);

    // Age the key past the threshold → the same call now bumps it.
    const realNow = Date.now;
    try {
      const later = realNow() + 61_000;
      Date.now = () => later;
      act(() => { invalidateIfStale(60_000, 'stats'); });
      expect(result.current).toBe(bumped + 1);
    } finally {
      Date.now = realNow;
    }
  });
});
