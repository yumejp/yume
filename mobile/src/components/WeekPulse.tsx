import { StyleSheet, Text, View } from 'react-native';
import type { DashboardStats } from '../api/types';
import { localISO } from '../data/live';
import { colors, font, radius, spacing } from '../theme';

/**
 * Home-screen pulse panel: the last 7 days of review activity and the next
 * 7 days of due load, in the Progress screen's visual language (indigo alpha
 * ramp = intensity, today accented). Renders its full frame from null stats
 * so the idle screen's no-layout-shift contract holds — cells and bars fill
 * in when /stats lands instead of inserting a panel and shoving the CTA.
 */
export function WeekPulse({ stats }: { stats: DashboardStats | null }) {
  const today = new Date();
  const byDate = new Map((stats?.heatmap ?? []).map((d) => [d.date, d.count]));
  const past = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(today);
    d.setDate(today.getDate() - (6 - i));
    return {
      key: localISO(d),
      letter: 'SMTWTFS'[d.getDay()],
      count: byDate.get(localISO(d)) ?? 0,
      isToday: i === 6,
    };
  });
  const pastMax = Math.max(1, ...past.map((d) => d.count));

  const forecastByDate = new Map((stats?.due_forecast ?? []).map((d) => [d.date, d.count]));
  const next = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(today);
    d.setDate(today.getDate() + i);
    return {
      key: localISO(d),
      letter: 'SMTWTFS'[d.getDay()],
      count: forecastByDate.get(localISO(d)) ?? 0,
    };
  });
  // Anything already overdue counts toward "today", same as the Progress tab.
  if (stats) next[0].count = Math.max(next[0].count, stats.totals.due_now);
  const nextMax = Math.max(1, ...next.map((d) => d.count));
  const weekDue = next.reduce((n, d) => n + d.count, 0);
  // Never-seen cards wait for Learn new words; they are not due, so they sit
  // beside the forecast rather than in it.
  const newWaiting = stats?.totals.new_waiting ?? 0;

  return (
    <View style={styles.panel}>
      <View style={styles.rowHeader}>
        <Text style={styles.rowLabel}>Last 7 days</Text>
        <Text style={styles.rowValue}>
          {stats ? `${stats.reviews_today ?? 0} ${(stats.reviews_today ?? 0) === 1 ? 'review' : 'reviews'} today` : ' '}
        </Text>
      </View>
      <View style={styles.cols}>
        {past.map((d) => (
          <View key={d.key} style={styles.col}>
            <View
              style={[
                styles.cell,
                { backgroundColor: heatColor(d.count, pastMax) },
                d.isToday && styles.cellToday,
              ]}
            />
            <Text style={[styles.colLabel, d.isToday && styles.colLabelToday]}>{d.letter}</Text>
          </View>
        ))}
      </View>
      <View style={styles.divider} />
      <View style={styles.rowHeader}>
        <Text style={styles.rowLabel}>Coming up</Text>
        <Text style={styles.rowValue}>
          {stats ? `${weekDue} due this week${newWaiting > 0 ? ` · ${newWaiting} new waiting` : ''}` : ' '}
        </Text>
      </View>
      <View style={styles.cols}>
        {next.map((d, i) => (
          <View key={d.key} style={styles.col}>
            <Text style={styles.barCount}>{d.count > 0 ? d.count : ''}</Text>
            <View style={styles.barTrack}>
              <View
                style={[
                  styles.bar,
                  {
                    height: `${Math.max(d.count > 0 ? 10 : 0, (d.count / nextMax) * 100)}%`,
                    backgroundColor: i === 0 ? colors.accent : `rgba(${colors.accentRgb}, 0.35)`,
                  },
                ]}
              />
            </View>
            <Text style={[styles.colLabel, i === 0 && styles.colLabelToday]}>{d.letter}</Text>
          </View>
        ))}
      </View>
    </View>
  );
}

/** Indigo alpha ramp — same encoding as the Progress heatmap. */
function heatColor(count: number, max: number): string {
  if (count === 0) return colors.surface2;
  const t = Math.min(1, count / max);
  return `rgba(${colors.accentRgb}, ${(0.25 + t * 0.75).toFixed(2)})`;
}

const styles = StyleSheet.create({
  panel: {
    alignSelf: 'stretch',
    marginTop: spacing.lg,
    paddingHorizontal: spacing.xs,
    gap: spacing.sm,
  },
  rowHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
  },
  rowLabel: {
    fontSize: font.xs,
    fontWeight: '600',
    color: colors.textTertiary,
  },
  rowValue: {
    fontSize: font.xs,
    color: colors.textSecondary,
  },
  cols: {
    flexDirection: 'row',
    gap: spacing.xs,
  },
  col: {
    flex: 1,
    alignItems: 'center',
    gap: 4,
  },
  cell: {
    alignSelf: 'stretch',
    height: 22,
    borderRadius: radius.sm - 2,
  },
  cellToday: {
    borderWidth: 1,
    borderColor: colors.accent,
  },
  colLabel: {
    fontSize: 10,
    color: colors.textTertiary,
  },
  // Today keeps its weekday letter — accent + bold, with the cell ring and
  // solid bar doing the real marking. (A 今 kanji was tried here and reads
  // as a smudged icon at label size, not a character.)
  colLabelToday: {
    color: colors.accent,
    fontWeight: '700',
  },
  divider: {
    alignSelf: 'stretch',
    height: 1,
    backgroundColor: colors.border,
    marginVertical: 2,
  },
  barCount: {
    fontSize: 10,
    color: colors.textSecondary,
    height: 13,
  },
  barTrack: {
    alignSelf: 'stretch',
    height: 30,
    justifyContent: 'flex-end',
    borderRadius: radius.sm - 2,
    backgroundColor: colors.surface2,
    overflow: 'hidden',
  },
  bar: {
    borderRadius: radius.sm - 2,
  },
});
