import type { DueCard } from '../../api/types';
import { dueCardToVM, formatWhen, gatedDueCount, glossLine, hostOf, vocabRowToCard } from '../live';

describe('formatWhen', () => {
  const now = Math.floor(Date.now() / 1000);
  it('is empty for missing timestamps', () => {
    expect(formatWhen(undefined)).toBe('');
    expect(formatWhen(0)).toBe('');
  });
  it('clamps the past to <1m (never negative)', () => {
    expect(formatWhen(now - 500)).toBe('<1m');
  });
  it('measures from the given reference time, not the render time', () => {
    const fetched = now - 40 * 60;
    expect(formatWhen(fetched + 24 * 3600, fetched)).toBe('1d');
    expect(formatWhen(fetched + 24 * 3600)).toBe('23h');
  });
  it('formats each magnitude', () => {
    expect(formatWhen(now + 10 * 60)).toBe('10m');
    expect(formatWhen(now + 5 * 3600)).toBe('5h');
    expect(formatWhen(now + 3 * 86400)).toBe('3d');
    expect(formatWhen(now + 60 * 86400)).toBe('2mo');
    expect(formatWhen(now + 2 * 365 * 86400)).toBe('2.0y');
  });
});

describe('glossLine', () => {
  it('joins senses with a dot separator, capped at three', () => {
    expect(glossLine([['work', 'job'], ['effort'], ['x'], ['dropped']])).toBe(
      'work, job · effort · x',
    );
  });
  it('is empty for absent glosses (Go omitempty)', () => {
    expect(glossLine(undefined)).toBe('');
    expect(glossLine([])).toBe('');
  });
});

describe('hostOf', () => {
  it('extracts the host', () => {
    expect(hostOf('http://192.0.2.7:7654')).toBe('192.0.2.7');
    expect(hostOf('https://example.com/watch?v=x')).toBe('example.com');
  });
  it('passes through non-URLs and handles null', () => {
    expect(hostOf('not a url')).toBe('not a url');
    expect(hostOf(null)).toBeUndefined();
  });
});

describe('dueCardToVM', () => {
  it('survives omitempty-absent arrays', () => {
    // The Go server omits empty arrays entirely; the wire type says they can
    // be absent even though the TS type declares them. This is the crash class
    // that once broke the client mapper.
    const wire = { id: 7, lemma: 'だんだん', card_type: 'vocab' } as unknown as DueCard;
    const vm = dueCardToVM(wire);
    expect(vm.lemma).toBe('だんだん');
    expect(vm.glosses).toBe('');
    expect(vm.intervals).toBeUndefined();
    expect(vm.isNew).toBeUndefined();
  });

  it('the LEMMA is the headword (kanji never overrides it) and previews map to labels', () => {
    const now = Math.floor(Date.now() / 1000);
    const wire: DueCard = {
      id: 1,
      lemma: '仕事',
      kanji: ['仕事'],
      readings: ['しごと'],
      glosses: [['work']],
      card_type: 'vocab',
      image_path: 'cards/1/frame.jpg',
      is_new: true,
      preview: { again: now + 60, hard: now + 300, good: now + 600, easy: now + 2 * 86400 },
    } as DueCard;
    const vm = dueCardToVM(wire);
    expect(vm.lemma).toBe('仕事');
    expect(vm.alsoWritten).toBeUndefined(); // kanji headword → nothing to add
    expect(vm.reading).toBe('しごと');
    expect(vm.isNew).toBe(true);
    expect(vm.imagePath).toBe('cards/1/frame.jpg');
    expect(vm.intervals).toEqual({ again: '1m', hard: '5m', good: '10m', easy: '2d' });
    expect(vm.mature).toBe(false);
  });

  it('keeps a usually-kana headword in kana and surfaces the kanji as "also written"', () => {
    // こだわり's dictionary forms are 拘り/拘わり — rare in the wild. The old
    // kanji[0] preference flashed 拘り on the front; the curated lemma wins.
    const wire = {
      id: 5, lemma: 'こだわり', kanji: ['拘り', '拘わり'], readings: ['こだわり'], card_type: 'vocab',
    } as unknown as DueCard;
    const vm = dueCardToVM(wire);
    expect(vm.lemma).toBe('こだわり');
    expect(vm.alsoWritten).toEqual(['拘り', '拘わり']);
    // Vocab rows follow the same contract.
    expect(vocabRowToCard({ lemma: 'くしゃみ', kanji: ['嚏'], status: 1 } as any, 0).lemma).toBe('くしゃみ');
  });

  it('marks a card mature when Hard would schedule it >21 days out', () => {
    const now = Math.floor(Date.now() / 1000);
    const wire = {
      id: 3,
      lemma: 'x',
      card_type: 'vocab',
      preview: { again: now + 600, hard: now + 30 * 86400, good: now + 60 * 86400, easy: now + 120 * 86400 },
    } as DueCard;
    expect(dueCardToVM(wire).mature).toBe(true);
    // No preview (offline Again lap, sample deck) → unknown maturity, not mature.
    expect(dueCardToVM({ id: 4, lemma: 'y', card_type: 'vocab' } as DueCard).mature).toBeUndefined();
  });

  it('strips markup from mined sentences', () => {
    const wire = {
      id: 2,
      lemma: 'x',
      card_type: 'vocab',
      sentence: '猫が<br>寝る',
    } as unknown as DueCard;
    expect(dueCardToVM(wire).sentence).toBe('猫が 寝る');
  });
});

describe('vocabRowToCard', () => {
  it('gives statuses-without-cards a stable negative key', () => {
    const card = vocabRowToCard({ lemma: '猫', status: 2 } as any, 4);
    expect(card.id).toBe(-5);
    expect(card.status).toBe('known');
    expect(card.glosses).toEqual([]);
  });
});

describe('gatedDueCount', () => {
  const stats = (over: object) =>
    ({ total: 10, due: 8, db_path: '', ...over }) as Parameters<typeof gatedDueCount>[0];

  it('falls back to raw due against an older server (no split fields)', () => {
    expect(gatedDueCount(stats({}))).toBe(8);
  });

  it('counts ONLY due reviews — new words are on-request and never promised', () => {
    // "8 due" = 2 reviews + 6 pending new: a plain session delivers exactly
    // the 2 reviews, so that's the honest number everywhere.
    const s = stats({ due_reviews: 2, due_new: 6, new_introduced_today: 0 });
    expect(gatedDueCount(s)).toBe(2);
    expect(gatedDueCount(stats({ due_reviews: 0, due_new: 6 }))).toBe(0);
  });

  it("promises today's catch-up share, not the whole backlog", () => {
    const catch_up = { backlog: 426, target_today: 142, done_today: 40, remaining_today: 102, days_left: 3 };
    expect(gatedDueCount(stats({ due_reviews: 426, catch_up }))).toBe(102);
  });

  it('still counts reviews on vacation (they can be done anyway)', () => {
    expect(gatedDueCount(stats({ due_reviews: 300, vacation_since: 1_790_000_000 }))).toBe(300);
  });
});
