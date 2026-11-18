import type { IconName } from '../components/Icon';

/** Expand a compact interval label ("3d") for screen readers ("3 days"). */
export function spokenInterval(label: string): string {
  if (label === '<1m') return 'under a minute';
  const m = label.match(/^([\d.]+)(mo|m|h|d|y)$/);
  if (!m) return label;
  const units: Record<string, string> = { m: 'minute', h: 'hour', d: 'day', mo: 'month', y: 'year' };
  const unit = units[m[2]] ?? m[2];
  return `${m[1]} ${unit}${Number(m[1]) === 1 ? '' : 's'}`;
}

/** Session progress as a waxing moon — full at session complete. */
export function moonPhaseIcon(pct: number): IconName {
  if (pct >= 95) return 'moon-full';
  if (pct >= 70) return 'moon-waxing-gibbous';
  if (pct >= 45) return 'moon-first-quarter';
  if (pct >= 15) return 'moon-waxing-crescent';
  return 'moon-new';
}
