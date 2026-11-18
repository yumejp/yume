/** Convert katakana to hiragana while leaving other characters unchanged. */
export function kataToHira(text: string): string {
  let result = '';
  for (const character of text) {
    const code = character.charCodeAt(0);
    result += code >= 0x30a1 && code <= 0x30f6
      ? String.fromCharCode(code - 0x60)
      : character;
  }
  return result;
}
