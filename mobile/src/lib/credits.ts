/** Data credits shown wherever the data appears. EDRDG's license asks for the
 *  acknowledgement on each screen that displays its entries, and links to its
 *  license; About carries the links. See LICENSES.md. */
export const JMDICT_CREDIT = 'JMdict © EDRDG, CC BY-SA 4.0';
export const EXAMPLES_CREDIT = 'Examples: Tanaka Corpus via Tatoeba, CC BY 2.0 FR';

export const CREDIT_LINKS: { label: string; url: string }[] = [
  { label: 'EDRDG license', url: 'https://www.edrdg.org/edrdg/licence.html' },
  { label: 'JMdict project', url: 'https://www.edrdg.org/jmdict/j_jmdict.html' },
  { label: 'KANJIDIC project', url: 'https://www.edrdg.org/wiki/index.php/KANJIDIC_Project' },
  { label: 'Tanaka Corpus', url: 'https://www.edrdg.org/wiki/index.php/Tanaka_Corpus' },
  { label: 'Tatoeba', url: 'https://tatoeba.org' },
];

/** The server's license notices page (LICENSES.md plus every bundled
 *  dependency's license). */
export function licensesUrl(serverUrl: string): string {
  return `${serverUrl.replace(/\/+$/, '')}/licenses`;
}
