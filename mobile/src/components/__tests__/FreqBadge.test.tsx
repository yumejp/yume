import { freqInfo } from '../FreqBadge';

describe('freqInfo', () => {
  it('calls an ichi1/news1 word common whatever its newspaper band says', () => {
    // 食べる is ichi1 + nf25; the band alone read as "top 13k".
    expect(freqInfo(25, true)?.label).toBe('common');
    expect(freqInfo(60)?.label).toBe('common');
  });

  it('rounds newspaper bands up to the Library bands, not to the nearest thousand', () => {
    expect(freqInfo(2)?.label).toBe('top 1k');
    expect(freqInfo(5)?.label).toBe('top 3k');
    expect(freqInfo(7)?.label).toBe('top 6k');
    expect(freqInfo(24)?.label).toBe('top 12k');
    expect(freqInfo(25)?.label).toBe('12k+');
  });

  it('shows nothing when the rank is unknown', () => {
    expect(freqInfo(undefined)).toBeNull();
    expect(freqInfo(0)).toBeNull();
  });
});
