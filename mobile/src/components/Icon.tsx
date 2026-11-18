import type { ReactNode } from 'react';
import type { StyleProp, ViewStyle } from 'react-native';
import Svg, { Circle, Line, Path, Polygon, Polyline, Rect } from 'react-native-svg';
import { colors } from '../theme';

/** House icon set: 24px grid, 2px strokes, round caps and joins. Add each new
 * name to both IconName and GLYPHS so TypeScript checks every call site. */

export type IconName =
  | 'alert-circle-outline'
  | 'book-open-page-variant-outline'
  | 'book-search-outline'
  | 'calendar-check-outline'
  | 'cards-outline'
  | 'check'
  | 'chevron-down'
  | 'chevron-up'
  | 'close'
  | 'cog-outline'
  | 'dots-vertical'
  | 'eye-outline'
  | 'fire'
  | 'flag-variant-outline'
  | 'microphone-outline'
  | 'moon-first-quarter'
  | 'moon-full'
  | 'moon-new'
  | 'moon-waxing-crescent'
  | 'moon-waxing-gibbous'
  | 'pencil-outline'
  | 'playlist-edit'
  | 'plus'
  | 'repeat'
  | 'restore'
  | 'robot-happy-outline'
  | 'sprout'
  | 'star-face'
  | 'star-four-points'
  | 'star-four-points-outline'
  | 'stop'
  | 'target'
  | 'trash-can-outline'
  | 'undo-variant'
  | 'volume-high';

type MoonName =
  | 'moon-new'
  | 'moon-waxing-crescent'
  | 'moon-first-quarter'
  | 'moon-waxing-gibbous'
  | 'moon-full';

/** Session and day progress as a waxing moon. */
const MOON_FRACTION: Record<MoonName, number> = {
  'moon-new': 0,
  'moon-waxing-crescent': 0.25,
  'moon-first-quarter': 0.5,
  'moon-waxing-gibbous': 0.78,
  'moon-full': 1,
};

/** Right-lit waxing moon: dim outline circle + a lit lune between the right
 *  semicircle and a terminator ellipse whose x-radius sweeps with `f`. */
function MoonPhase({ f }: { f: number }) {
  const r = 10;
  const rx = Math.abs(1 - 2 * f) * r;
  const sweep = f < 0.5 ? 0 : 1;
  return (
    <>
      <Circle cx="12" cy="12" r={r} opacity={0.4} />
      {f > 0 && (
        <Path
          d={`M12 2a${r} ${r} 0 0 1 0 ${2 * r}a${rx} ${r} 0 0 ${sweep} 0 -${2 * r}`}
          fill="currentColor"
          stroke="none"
        />
      )}
    </>
  );
}

/** Continuous waxing moon for progress meters (today's reviews vs the
 *  daily target). Same terminator math as the quantized moon-* names. */
export function MoonProgress({
  f,
  size = 16,
  color = colors.textPrimary,
}: {
  f: number;
  size?: number;
  color?: string;
}) {
  const clamped = Math.max(0, Math.min(1, Number.isFinite(f) ? f : 0));
  return (
    <Svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      color={color}
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <MoonPhase f={clamped} />
    </Svg>
  );
}

/* eslint-disable react/jsx-key */
const GLYPHS: Record<Exclude<IconName, MoonName>, ReactNode> = {
  'alert-circle-outline': (
    <>
      <Circle cx="12" cy="12" r="9" />
      <Line x1="12" y1="8" x2="12" y2="12.5" />
      <Path d="M12 16v.1" />
    </>
  ),
  'book-open-page-variant-outline': (
    <>
      <Path d="M2 4.5h6a4 4 0 0 1 4 4V20a3 3 0 0 0-3-3H2z" />
      <Path d="M22 4.5h-6a4 4 0 0 0-4 4V20a3 3 0 0 1 3-3h7z" />
    </>
  ),
  'book-search-outline': (
    <>
      <Path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5V4.5A2.5 2.5 0 0 1 6.5 2z" />
      <Path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
      <Circle cx="11.6" cy="9.2" r="2.8" />
      <Path d="m13.7 11.3 2.3 2.3" />
    </>
  ),
  'calendar-check-outline': (
    <>
      <Rect x="3" y="5" width="18" height="16" rx="2" />
      <Path d="M16 3v4M8 3v4M3 10h18" />
      <Path d="m9 15.5 2 2 4-4" />
    </>
  ),
  'cards-outline': (
    <>
      <Rect x="3" y="7.5" width="14.5" height="12" rx="2" />
      <Path d="M7.5 4h11A2.5 2.5 0 0 1 21 6.5V16" />
    </>
  ),
  check: <Path d="M20 6 9 17l-5-5" />,
  'chevron-down': <Path d="m6 9 6 6 6-6" />,
  'chevron-up': <Path d="m6 15 6-6 6 6" />,
  close: <Path d="M18 6 6 18M6 6l12 12" />,
  'cog-outline': (
    <>
      <Circle cx="12" cy="12" r="3" />
      <Path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </>
  ),
  'dots-vertical': (
    <>
      <Circle cx="12" cy="5" r="1.4" fill="currentColor" stroke="none" />
      <Circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none" />
      <Circle cx="12" cy="19" r="1.4" fill="currentColor" stroke="none" />
    </>
  ),
  'eye-outline': (
    <>
      <Path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <Circle cx="12" cy="12" r="3" />
    </>
  ),
  fire: (
    <Path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z" />
  ),
  'microphone-outline': (
    <>
      <Path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3z" />
      <Path d="M19 11v1a7 7 0 0 1-14 0v-1" />
      <Path d="M12 19v3" />
    </>
  ),
  'flag-variant-outline': (
    <>
      <Path d="M5 21V4" />
      <Path d="M5 4c4-2 7 2 11 0v9c-4 2-7-2-11 0" />
    </>
  ),
  'pencil-outline': (
    <>
      <Path d="M12 20h9" />
      <Path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
    </>
  ),
  'playlist-edit': (
    <>
      <Path d="M3 6.5h11M3 11h11M3 15.5h6" />
      <Path d="M20.2 12.3a1.9 1.9 0 0 1 2.7 2.7L17 20.9l-3.5.8.8-3.5z" />
    </>
  ),
  plus: <Path d="M12 5v14M5 12h14" />,
  repeat: (
    <>
      <Path d="m17 2 4 4-4 4" />
      <Path d="M3 11v-1a4 4 0 0 1 4-4h14" />
      <Path d="m7 22-4-4 4-4" />
      <Path d="M21 13v1a4 4 0 0 1-4 4H3" />
    </>
  ),
  restore: (
    <>
      <Polyline points="1 4 1 10 7 10" />
      <Path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
    </>
  ),
  'robot-happy-outline': (
    <>
      <Rect x="4" y="8.5" width="16" height="10.5" rx="3" />
      <Path d="M12 8.5V5.5" />
      <Circle cx="12" cy="4" r="1.2" fill="currentColor" stroke="none" />
      <Path d="M8.8 12.6v.1M15.2 12.6v.1" />
      <Path d="M9.3 15.6c1.6 1.3 3.8 1.3 5.4 0" />
      <Path d="M2 11.5v3M22 11.5v3" />
    </>
  ),
  sprout: (
    <>
      <Path d="M7 20h10" />
      <Path d="M10 20c5.5-2.5.8-6.4 3-10" />
      <Path d="M9.5 9.4c1.1.8 1.8 2.2 2.3 3.7-2 .4-3.5.4-4.8-.3-1.2-.6-2.3-1.9-3-4.2 2.8-.5 4.4 0 5.5.8z" />
      <Path d="M14.1 6a7 7 0 0 0-1.1 4c1.9-.1 3.3-.6 4.3-1.4 1-1 1.6-2.3 1.7-4.6-2.7.1-4 1-4.9 2z" />
    </>
  ),
  'star-face': (
    <Polygon
      points="12 2.5 15 8.8 22 9.8 17 14.6 18.2 21.5 12 18.2 5.8 21.5 7 14.6 2 9.8 9 8.8"
      fill="currentColor"
      stroke="none"
    />
  ),
  'star-four-points': (
    <Path d="M12 2.5 14.3 9.7 21.5 12l-7.2 2.3L12 21.5 9.7 14.3 2.5 12l7.2-2.3z" fill="currentColor" />
  ),
  'star-four-points-outline': (
    <Path d="M12 2.5 14.3 9.7 21.5 12l-7.2 2.3L12 21.5 9.7 14.3 2.5 12l7.2-2.3z" />
  ),
  stop: <Rect x="6.5" y="6.5" width="11" height="11" rx="1.5" fill="currentColor" stroke="none" />,
  target: (
    <>
      <Circle cx="12" cy="12" r="9" />
      <Circle cx="12" cy="12" r="5" />
      <Circle cx="12" cy="12" r="1" fill="currentColor" stroke="none" />
    </>
  ),
  'trash-can-outline': (
    <>
      <Path d="M3 6h18" />
      <Path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
      <Path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
      <Line x1="10" y1="11" x2="10" y2="17" />
      <Line x1="14" y1="11" x2="14" y2="17" />
    </>
  ),
  'undo-variant': (
    <>
      <Polyline points="1 4 1 10 7 10" />
      <Path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
    </>
  ),
  'volume-high': (
    <>
      <Polygon points="11 5 6 9 3 9 3 15 6 15 11 19" />
      <Path d="M15.2 8.8a4.6 4.6 0 0 1 0 6.4" />
      <Path d="M18.5 5.5a9.2 9.2 0 0 1 0 13" opacity={0.5} />
    </>
  ),
};
/* eslint-enable react/jsx-key */

export function Icon({
  name,
  size = 16,
  color = colors.textPrimary,
  style,
}: {
  name: IconName;
  size?: number;
  color?: string;
  style?: StyleProp<ViewStyle>;
}) {
  const moonF = name in MOON_FRACTION ? MOON_FRACTION[name as MoonName] : undefined;
  return (
    <Svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      color={color}
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={style}
    >
      {moonF !== undefined ? <MoonPhase f={moonF} /> : GLYPHS[name as Exclude<IconName, MoonName>]}
    </Svg>
  );
}
