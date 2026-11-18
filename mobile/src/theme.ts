import { Platform } from 'react-native';

/**
 * Yume design tokens. Every colour in the app comes from here.
 */



/** Warm paper, plum ink, and coral actions. */
export const colors = {
  surface0: '#F7EEE6',
  surface1: '#FCF6F0',
  surface2: '#EDE1D8',
  textPrimary: '#3A2436',
  textSecondary: '#7A6172',
  textTertiary: '#A38D9A',
  accent: '#E27864',
  accentRgb: '226, 120, 100',
  accentMuted: 'rgba(226, 120, 100, 0.14)',
  onAccent: '#FFF8F1',
  moon: '#F2C35E',
  scheme: 'light',
  accentText: '#B8503E',
  border: 'rgba(58, 36, 54, 0.12)',
  backdrop: 'rgba(58, 36, 54, 0.35)',
  statusKnown: '#4E9C6C',
  statusLearning: '#C4892A',
  statusUnknown: '#C9556B',
  statusIgnored: '#A38D9A',
};

export type Palette = typeof colors;

/** Display face for brand moments only — the poster headline and hero number.
 *  System mincho: Hiragino on
 *  iOS, the platform serif (Noto Serif CJK) on Android — zero font bytes
 *  shipped. Never body text, never anything the learner reads repeatedly.
 *  The web stack names Japanese mincho faces: a bare 'serif' lets kanji fall
 *  back to a Chinese font with Chinese glyph forms (直, 骨). */
export const fontDisplay = Platform.select({
  ios: 'Hiragino Mincho ProN',
  web: '"Hiragino Mincho ProN", "Yu Mincho", "Noto Serif JP", serif',
  default: 'serif',
});

/** Japanese body text on the web: Japanese system gothics first, so a word
 *  never mixes a Chinese fallback font with a Japanese one. Native apps keep
 *  the platform font, which already follows the text's language. */
export const fontJa = Platform.select({
  web: '"Hiragino Sans", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", Meiryo, "Noto Sans JP", sans-serif',
  default: undefined,
});

/** Spread onto Japanese Text and containers: `lang="ja"` on the web, which
 *  react-native-web renders as the DOM attribute. Browsers then pick Japanese
 *  glyph forms and screen readers a Japanese voice; the UI chrome stays
 *  `lang="en"`. React Native's types lack `lang`, and native gets nothing. */
export const langJa: object = Platform.OS === 'web' ? { lang: 'ja' } : {};

/** Controls use md, surfaces lg; pill is for the primary button and count
 *  badges only. */
export const radius = {
  sm: 8,
  md: 10,
  lg: 16,
  pill: 999,
} as const;

export const spacing = {
  xs: 4,
  sm: 8,
  md: 16,
  lg: 24,
  xl: 32,
} as const;

// Sized for a phone held at arm's length. md tracks platform body conventions
// (Material 16sp / iOS 17pt); everything else moves with it.
export const font = {
  xs: 12,
  sm: 14,
  md: 16,
  lg: 18,
  xl: 24,
  display: 36,
  /** Japanese running text — kanji carry more detail than Latin at equal
   *  size, so JP body copy starts a step above `md`. */
  jp: 18,
} as const;

export type StatusColor =
  | typeof colors.statusKnown
  | typeof colors.statusLearning
  | typeof colors.statusUnknown
  | typeof colors.statusIgnored;

/** Word-status → Okabe-Ito color. */
export function statusColor(
  status: 'unknown' | 'learning' | 'known' | 'ignored',
): StatusColor {
  switch (status) {
    case 'known':
      return colors.statusKnown;
    case 'learning':
      return colors.statusLearning;
    case 'ignored':
      return colors.statusIgnored;
    default:
      return colors.statusUnknown;
  }
}
