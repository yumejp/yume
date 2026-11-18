export interface ReadingPart {
  kanji: string;
  reading: string;
}

/** A column under the headword: a kanji with its reading, or kana as written. */
export type SplitColumn = { kanji: string; reading: string } | { kana: string };

const KANJI = /[㐀-䶿一-鿿々]/;

/** Lays the server's per-kanji reading parts out along the word, keeping
 *  okurigana as written (潰れる → 潰 つぶ, れる). Null when they don't fit. */
export function splitColumns(word: string, parts: ReadingPart[] | undefined): SplitColumn[] | null {
  if (!parts?.length) return null;
  const out: SplitColumn[] = [];
  let next = 0;
  for (const ch of Array.from(word)) {
    if (KANJI.test(ch)) {
      const p = parts[next++];
      if (!p || p.kanji !== ch) return null;
      out.push({ kanji: p.kanji, reading: p.reading });
    } else {
      const last = out[out.length - 1];
      if (last && 'kana' in last) last.kana += ch;
      else out.push({ kana: ch });
    }
  }
  return next === parts.length ? out : null;
}
