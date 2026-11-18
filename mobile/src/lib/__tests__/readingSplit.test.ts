import { splitColumns } from '../readingSplit';

describe('splitColumns', () => {
  it('pairs each kanji with its reading', () => {
    expect(splitColumns('供給', [{ kanji: '供', reading: 'きょう' }, { kanji: '給', reading: 'きゅう' }])).toEqual([
      { kanji: '供', reading: 'きょう' },
      { kanji: '給', reading: 'きゅう' },
    ]);
  });

  it('keeps okurigana as written', () => {
    expect(splitColumns('潰れる', [{ kanji: '潰', reading: 'つぶ' }])).toEqual([
      { kanji: '潰', reading: 'つぶ' },
      { kana: 'れる' },
    ]);
  });

  it('refuses parts that do not fit the word', () => {
    expect(splitColumns('供給', [{ kanji: '供', reading: 'きょう' }])).toBeNull();
    expect(splitColumns('供給', undefined)).toBeNull();
  });
});
