import { isKana, isMixed, isRomaji, toHiragana } from 'wanakana';

/**
 * Build dictionary lookup queries from raw search input. Kana/kanji passes
 * through untouched; romaji, and romaji mixed with kana (a keyboard switched
 * mid-word), is converted to hiragana so it matches JMdict readings without
 * an IME. Full-width letters from a Japanese IME are folded to ASCII first.
 *
 * Romaji "n" before a vowel or y is ambiguous: wanakana always takes the
 * な-row reading ("renai" → れない), but the typist may have meant syllabic
 * ん — れんあい (恋愛). Getting ん explicitly needs an apostrophe ("ren'ai"),
 * a convention nobody uses on a phone keyboard, so every ambiguous n is
 * expanded BOTH ways and all readings are searched. The as-typed reading
 * stays first so its results rank ahead.
 *
 * `kana` is the hint shown under the search box. It is set only when every
 * reading converted to pure kana: an English word like "study" leaves Latin
 * letters behind ("sつdy"), and that garble is neither searched nor shown.
 *
 * The folding only feeds the romaji conversion. JMdict keeps thousands of
 * headwords in full width (Ｔシャツ, Ｘ線, ＣＤ, バツ１), and lookups are
 * exact, so input that folding changed is also searched exactly as typed.
 */
export function buildQueries(input: string): { queries: string[]; kana: string | null } {
  const raw = input.trim();
  const q = raw.normalize('NFKC');
  if (!q) return { queries: [], kana: null };
  const asTyped = (queries: string[]) => (raw === q || queries.includes(raw) ? queries : [...queries, raw]);
  if (!isRomaji(q) && !isMixed(q)) return { queries: asTyped([q]), kana: null };
  // Each ambiguous n doubles the variants — cap the fan-out at 8 queries.
  const spots = ambiguousNSpots(q).slice(0, 3);
  const kanas: string[] = [];
  for (let mask = 0; mask < 1 << spots.length; mask++) {
    let variant = q;
    // Insert right-to-left so earlier spot indices stay valid.
    for (let b = spots.length - 1; b >= 0; b--) {
      if (mask & (1 << b)) {
        variant = `${variant.slice(0, spots[b] + 1)}'${variant.slice(spots[b] + 1)}`;
      }
    }
    const kana = toHiragana(variant);
    if (kana && kana !== q && isKana(kana) && !kanas.includes(kana)) kanas.push(kana);
  }
  if (kanas.length === 0) return { queries: asTyped([q]), kana: null };
  return { queries: asTyped(kanas), kana: kanas.join('・') };
}

/**
 * The input as an English search, when it could be one: plain letters,
 * spaces, hyphens and apostrophes. Romaji qualifies too ("made" is both まで
 * and "made"), so callers search both ways and list Japanese hits first.
 */
export function englishQuery(input: string): string | null {
  const q = input.normalize('NFKC').trim();
  return /^[A-Za-z][A-Za-z' -]*$/.test(q) ? q : null;
}

/** Indices of each "n" that could read as either な-row or ん + vowel/y.
 *  Word-initial n is skipped (no ん-initial words) and "nn" is already ん. */
function ambiguousNSpots(q: string): number[] {
  const lower = q.toLowerCase();
  const spots: number[] = [];
  for (let i = 1; i < lower.length - 1; i++) {
    if (lower[i] === 'n' && 'aiueoy'.includes(lower[i + 1]) && lower[i - 1] !== 'n') {
      spots.push(i);
    }
  }
  return spots;
}
