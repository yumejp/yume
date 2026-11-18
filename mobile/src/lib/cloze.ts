/** Split a cloze card's sentence into text and blank parts. The blank is the
 *  first candidate found in the sentence: the surface form saved when the card
 *  was made (covers conjugations), then the headword, then the lemma. With no
 *  match the sentence is one text part, so the card still reads. */
export type ClozePart = { kind: 'text'; text: string } | { kind: 'blank'; text: string };

export function buildClozeParts(sentence: string, candidates: (string | undefined | null)[]): ClozePart[] {
  for (const c of candidates) {
    if (!c || !sentence.includes(c)) continue;
    const parts: ClozePart[] = [];
    let i = 0;
    while (i < sentence.length) {
      const idx = sentence.indexOf(c, i);
      if (idx < 0) {
        parts.push({ kind: 'text', text: sentence.slice(i) });
        break;
      }
      if (idx > i) parts.push({ kind: 'text', text: sentence.slice(i, idx) });
      parts.push({ kind: 'blank', text: c });
      i = idx + c.length;
    }
    return parts;
  }
  return [{ kind: 'text', text: sentence }];
}

/** The text a cloze hides, or undefined when nothing in the sentence matches. */
export function clozeTarget(parts: ClozePart[]): string | undefined {
  return parts.find((p) => p.kind === 'blank')?.text;
}
