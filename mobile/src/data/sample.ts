import type { Card, DayActivity, DeckStats } from './types';

/**
 * Sample deck shown until a server is connected. Every surface that renders
 * it says so. Sentences are original, so the deck can ship anywhere.
 */

export const SAMPLE_CARDS: Card[] = [
  {
    id: 1,
    lemma: '冒険',
    reading: 'ぼうけん',
    glosses: ['adventure', 'risky attempt'],
    sentence: '週末は山でちょっとした冒険をした。',
    sentenceTranslation: 'I had a little adventure in the mountains over the weekend.',
    source: 'Sample · travel vlog',
    tags: ['travel'],
    status: 'learning',
    dueInDays: 0,
  },
  {
    id: 2,
    lemma: '魔法',
    reading: 'まほう',
    glosses: ['magic', 'sorcery', 'witchcraft'],
    sentence: '魔法が使えたら、まず何をする？',
    sentenceTranslation: 'If you could use magic, what would you do first?',
    source: 'Sample · conversation',
    tags: ['anime'],
    status: 'learning',
    dueInDays: 0,
  },
  {
    id: 3,
    lemma: '懐かしい',
    reading: 'なつかしい',
    glosses: ['nostalgic', 'fondly remembered'],
    sentence: '昔の写真を見ると懐かしくなる。',
    sentenceTranslation: 'Looking at old photos makes me nostalgic.',
    source: 'Sample · conversation',
    tags: ['everyday'],
    status: 'learning',
    dueInDays: -1,
  },
  {
    id: 4,
    lemma: '天気',
    reading: 'てんき',
    glosses: ['weather', 'fair weather'],
    sentence: '今日は良い天気ですね。',
    sentenceTranslation: 'Nice weather today, isn’t it?',
    source: 'Sample · weather report',
    tags: ['jlpt-n5', 'news'],
    status: 'known',
    dueInDays: 12,
  },
  {
    id: 5,
    lemma: '約束',
    reading: 'やくそく',
    glosses: ['promise', 'agreement', 'appointment'],
    sentence: '友達と週末に会う約束をした。',
    sentenceTranslation: 'I made plans to meet a friend this weekend.',
    source: 'Sample · conversation',
    tags: ['jlpt-n4'],
    status: 'learning',
    dueInDays: 0,
  },
  {
    id: 6,
    lemma: '手紙',
    reading: 'てがみ',
    glosses: ['letter (written message)'],
    sentence: '祖母に手紙を書いた。',
    sentenceTranslation: 'I wrote a letter to my grandmother.',
    source: 'Sample · diary',
    tags: ['jlpt-n5'],
    status: 'known',
    dueInDays: 21,
  },
  {
    id: 7,
    lemma: '勇者',
    reading: 'ゆうしゃ',
    glosses: ['hero', 'brave person'],
    sentence: '勇者は仲間と一緒に旅に出た。',
    sentenceTranslation: 'The hero set out on a journey with their companions.',
    source: 'Sample · game',
    tags: ['anime', 'games'],
    status: 'learning',
    dueInDays: 1,
  },
  {
    id: 8,
    lemma: '頑張る',
    reading: 'がんばる',
    glosses: ['to persevere', 'to do one’s best'],
    sentence: '明日も頑張りましょう。',
    sentenceTranslation: 'Let’s do our best tomorrow too.',
    source: 'Sample · morning news',
    tags: ['jlpt-n5'],
    status: 'known',
    dueInDays: 30,
  },
  {
    id: 9,
    lemma: '寂しい',
    reading: 'さびしい',
    glosses: ['lonely', 'desolate'],
    sentence: '引っ越してから、少し寂しい。',
    sentenceTranslation: 'Since moving, I’ve been a little lonely.',
    source: 'Sample · diary',
    tags: ['jlpt-n4'],
    status: 'learning',
    dueInDays: 0,
  },
  {
    id: 10,
    lemma: '景色',
    reading: 'けしき',
    glosses: ['scenery', 'landscape', 'view'],
    sentence: 'ここからの景色は最高だ。',
    sentenceTranslation: 'The view from here is the best.',
    source: 'Sample · travel vlog',
    tags: ['travel'],
    status: 'learning',
    dueInDays: 2,
  },
  {
    id: 11,
    lemma: '無理',
    reading: 'むり',
    glosses: ['impossible', 'unreasonable', 'overdoing it'],
    sentence: '無理しないでね。',
    sentenceTranslation: 'Don’t push yourself too hard, okay?',
    source: 'Sample · conversation',
    tags: ['conversation'],
    status: 'known',
    dueInDays: 9,
  },
  {
    id: 12,
    lemma: '騒がしい',
    reading: 'さわがしい',
    glosses: ['noisy', 'boisterous'],
    sentence: '隣の部屋が騒がしくて眠れなかった。',
    sentenceTranslation: 'The next room was so noisy I couldn’t sleep.',
    source: 'Sample · diary',
    tags: ['jlpt-n3'],
    status: 'learning',
    dueInDays: 0,
  },
];

/** Deterministic pseudo-random for stable sample history (mulberry32). */
function mulberry32(seed: number): () => number {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function buildHistory(days: number): DayActivity[] {
  const rand = mulberry32(0x59554d45); // "YUME"
  const out: DayActivity[] = [];
  const today = new Date();
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setDate(today.getDate() - i);
    // ~1 in 5 rest days; otherwise 1–24 reviews, heavier on recent days.
    const rested = rand() < 0.2 && i > 2;
    const recencyBoost = 1 + (1 - i / days) * 1.5;
    const reviews = rested ? 0 : Math.ceil(rand() * 9 * recencyBoost);
    out.push({ date: d.toISOString().slice(0, 10), reviews });
  }
  return out;
}

const history = buildHistory(12 * 7);

function trailingStreak(h: DayActivity[]): number {
  let n = 0;
  for (let i = h.length - 1; i >= 0 && h[i].reviews > 0; i--) n++;
  return n;
}

export const SAMPLE_STATS: DeckStats = {
  total: SAMPLE_CARDS.length,
  due: SAMPLE_CARDS.filter((c) => c.dueInDays <= 0).length,
  learning: SAMPLE_CARDS.filter((c) => c.status === 'learning').length,
  known: SAMPLE_CARDS.filter((c) => c.status === 'known').length,
  streakDays: trailingStreak(history),
  history,
  dueForecast: [0, 1, 2, 3, 4, 5, 6].map(
    (day) => SAMPLE_CARDS.filter((c) => Math.max(0, c.dueInDays) === day).length,
  ),
};

export const SAMPLE_DUE_QUEUE: Card[] = SAMPLE_CARDS.filter(
  (c) => c.dueInDays <= 0,
).sort((a, b) => a.dueInDays - b.dueInDays);
