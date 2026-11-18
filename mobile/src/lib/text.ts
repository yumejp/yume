/**
 * Strip HTML markup and decode the handful of entities that leak in from mined
 * web sentences (e.g. a line break stored as a literal `<br>`). Without
 * this the tag renders verbatim on the card AND gets read aloud by TTS ("br").
 */
export function stripMarkup(s: string | undefined | null): string {
  if (!s) return '';
  return s
    .replace(/<br\s*\/?>/gi, ' ') // <br>, <br/>, <br /> → space
    .replace(/<\/?[a-z][^>]*>/gi, '') // any other tag
    .replace(/&nbsp;/gi, ' ')
    .replace(/&amp;/gi, '&')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&#39;|&apos;/gi, "'")
    .replace(/&quot;/gi, '"')
    .replace(/\s+/g, ' ')
    .trim();
}
