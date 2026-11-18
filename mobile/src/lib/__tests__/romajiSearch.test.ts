import { buildQueries, englishQuery } from '../romajiSearch';

describe('buildQueries', () => {
  test('empty input yields no queries', () => {
    expect(buildQueries('')).toEqual({ queries: [], kana: null });
    expect(buildQueries('   ')).toEqual({ queries: [], kana: null });
  });

  test('kana and kanji pass through untouched', () => {
    expect(buildQueries('ねこ')).toEqual({ queries: ['ねこ'], kana: null });
    expect(buildQueries('恋愛')).toEqual({ queries: ['恋愛'], kana: null });
  });

  test('unambiguous romaji converts to a single kana query', () => {
    expect(buildQueries('neko')).toEqual({ queries: ['ねこ'], kana: 'ねこ' });
    // n before a consonant is already ん — nothing to expand.
    expect(buildQueries('sensei')).toEqual({ queries: ['せんせい'], kana: 'せんせい' });
    expect(buildQueries('manga')).toEqual({ queries: ['まんが'], kana: 'まんが' });
  });

  test('ambiguous n searches both the な-row and ん readings', () => {
    // The bug: "renai" only ever searched れない — 恋愛 (れんあい) was unreachable.
    expect(buildQueries('renai')).toEqual({
      queries: ['れない', 'れんあい'],
      kana: 'れない・れんあい',
    });
    expect(buildQueries('zenin').queries).toContain('ぜんいん'); // 全員
  });

  test('n before y is ambiguous too', () => {
    expect(buildQueries('konya').queries).toEqual(['こにゃ', 'こんや']); // 今夜
  });

  test("an explicit apostrophe already means ん and isn't re-expanded", () => {
    expect(buildQueries("ren'ai")).toEqual({ queries: ['れんあい'], kana: 'れんあい' });
  });

  test('nn is already ん, not an ambiguity', () => {
    expect(buildQueries('konnya')).toEqual({ queries: ['こんにゃ'], kana: 'こんにゃ' });
  });

  test('word-initial n is not expanded', () => {
    expect(buildQueries('nani').queries).toEqual(['なに', 'なんい']); // only the inner n
  });

  test('fan-out is capped at 8 variants', () => {
    expect(buildQueries('nanananananana').queries.length).toBeLessThanOrEqual(8);
  });

  test('full-width romaji from a Japanese IME converts like ASCII', () => {
    expect(buildQueries('ｔａｂｅｒｕ')).toEqual({ queries: ['たべる', 'ｔａｂｅｒｕ'], kana: 'たべる' });
  });

  test('full-width headwords are still searched as typed', () => {
    // The regression: JMdict keys like Ｔシャツ are full width and lookups
    // are exact, so searching only the folded Tシャツ found nothing.
    expect(buildQueries('Ｔシャツ')).toEqual({ queries: ['Tシャツ', 'Ｔシャツ'], kana: null });
    expect(buildQueries('Ｘ線').queries).toContain('Ｘ線');
    expect(buildQueries('ＣＤプレーヤー').queries).toContain('ＣＤプレーヤー');
    expect(buildQueries('バツ１').queries).toContain('バツ１');
    expect(buildQueries(' ＣＤ ').queries).toContain('ＣＤ');
    // ＡＩ also reads as あい; both are searched, the reading first.
    expect(buildQueries('ＡＩ')).toEqual({ queries: ['あい', 'ＡＩ'], kana: 'あい' });
  });

  test('ASCII input is not searched twice', () => {
    expect(buildQueries('neko').queries).toEqual(['ねこ']);
    expect(buildQueries('Tシャツ').queries).toEqual(['Tシャツ']);
  });

  test('romaji mixed with kana converts the romaji part', () => {
    expect(buildQueries('tabeる')).toEqual({ queries: ['たべる'], kana: 'たべる' });
  });

  test('English words that leave Latin letters behind are not garbled', () => {
    // The bug: "study" showed "Searching sつdy".
    expect(buildQueries('study')).toEqual({ queries: ['study'], kana: null });
    expect(buildQueries('hello')).toEqual({ queries: ['hello'], kana: null });
    expect(buildQueries('give up')).toEqual({ queries: ['give up'], kana: null });
  });
});

describe('englishQuery', () => {
  test('plain letters can be an English search, romaji included', () => {
    expect(englishQuery(' study ')).toBe('study');
    expect(englishQuery('give up')).toBe('give up');
    expect(englishQuery('made')).toBe('made');
    expect(englishQuery('ｓｔｕｄｙ')).toBe('study');
  });

  test('Japanese and mixed input are not', () => {
    expect(englishQuery('ねこ')).toBeNull();
    expect(englishQuery('tabeる')).toBeNull();
    expect(englishQuery('')).toBeNull();
  });
});
