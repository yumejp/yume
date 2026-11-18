/** Friendly labels for the common JMdict POS codes (the rest fall through). */
const POS_LABELS: Record<string, string> = {
  n: 'noun', 'n-suf': 'noun suffix', 'n-pref': 'noun prefix', 'n-adv': 'adverbial noun',
  pn: 'pronoun',
  v1: 'ichidan verb', v5: 'godan verb', v5u: 'godan verb', v5k: 'godan verb',
  v5s: 'godan verb', v5t: 'godan verb', v5r: 'godan verb', v5g: 'godan verb',
  v5b: 'godan verb', v5m: 'godan verb', v5n: 'godan verb', vk: 'kuru verb',
  vs: 'suru verb', 'vs-i': 'suru verb', 'vs-s': 'suru verb',
  vi: 'intransitive', vt: 'transitive', 'aux-v': 'auxiliary verb', aux: 'auxiliary',
  'adj-i': 'i-adjective', 'adj-na': 'na-adjective', 'adj-no': 'adjective',
  'adj-f': 'adjective', 'adj-pn': 'pre-noun adjectival',
  adv: 'adverb', 'adv-to': 'adverb', exp: 'expression', int: 'interjection',
  conj: 'conjunction', prt: 'particle', pref: 'prefix', suf: 'suffix', ctr: 'counter',
};

/** Map an entry's POS codes to a readable, deduped label (e.g. "noun"). */
export function posLabel(pos?: string[]): string {
  if (!pos?.length) return '';
  const seen = new Set<string>();
  const out: string[] = [];
  for (const p of pos) {
    const label = POS_LABELS[p] ?? p;
    if (seen.has(label)) continue;
    seen.add(label);
    out.push(label);
  }
  return out.join(' · ');
}
