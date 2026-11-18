import type { ReactNode } from 'react';
import { useState } from 'react';
import { ActivityIndicator, RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { statsToDeckStats } from '../data/live';
import { useServerQuery } from '../data/queryStore';
import { SAMPLE_STATS } from '../data/sample';
import type { DayActivity, DeckStats } from '../data/types';
import { useServer } from '../server/ServerContext';
import { colors, font, radius, spacing } from '../theme';

export default function ProgressScreen() {
  const server = useServer();
  const live = server.live;

  const [refreshing, setRefreshing] = useState(false);

  // Refetches whenever 'stats' is invalidated (a rating, undo, card save or
  // delete anywhere in the app) — the tab shell keeps this screen mounted, so
  // without the invalidation hook it only ever loaded once.
  const statsQuery = useServerQuery('stats', live ? () => server.client!.stats() : null, [live]);
  const liveStats = statsQuery.data;
  const error = statsQuery.error;

  // Sample stats are used only when no server is configured.
  const s: DeckStats | null = live ? (liveStats ? statsToDeckStats(liveStats) : null) : SAMPLE_STATS;
  // Frequency bands also provide progress milestones.
  const bandsQuery = useServerQuery('vocab', live ? () => server.client!.vocabBands({}) : null, [live]);
  const bands = (bandsQuery.data ?? []).filter((b) => b.total > 0 || (b.size ?? 0) > 0);
  const longest = live && liveStats ? liveStats.streak.longest : 0;
  const streakTitle =
    s && s.streakDays > 0
      ? `${s.streakDays}-day streak`
      : longest > 0
        ? `Longest streak ${longest} ${longest === 1 ? 'day' : 'days'}`
        : 'Streak';

  return (
    <ScrollView
      contentContainerStyle={styles.scroll}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          onRefresh={() => {
            setRefreshing(true);
            statsQuery.reload();
            void server.refresh().finally(() => setRefreshing(false));
          }}
          tintColor={colors.textSecondary}
        />
      }
    >
      {error ? <Text style={styles.error}>{error}</Text> : null}

      {!s ? (
        <View style={styles.loadingWrap}>
          {error ? (
            <Text style={styles.loadingText}>couldn’t load your stats · pull to retry</Text>
          ) : (
            <>
              <ActivityIndicator size="small" color={colors.accentText} />
              <Text style={styles.loadingText}>loading your stats…</Text>
            </>
          )}
        </View>
      ) : (
        <>
          <View style={styles.statGrid}>
            <StatCard label="Words" value={s.total} />
            <StatCard label="Due now" value={s.due} accent={colors.accentText} />
            <StatCard label="Learning" value={s.learning} accent={colors.statusLearning} />
            <StatCard label="Known" value={s.known} accent={colors.statusKnown} />
          </View>
          {s.bySound ? (
            <Text style={[styles.caption, styles.gridNote]}>
              {s.bySound} Learning {s.bySound === 1 ? 'word' : 'words'} known by sound · {s.bySound === 1 ? 'its card shows' : 'their cards show'} furigana
            </Text>
          ) : null}
          {s.newWaiting ? (
            <Text style={[styles.caption, styles.gridNote]}>
              {s.newWaiting} new {s.newWaiting === 1 ? 'word' : 'words'} waiting · Review offers them once your reviews are done
            </Text>
          ) : null}

          <Section title={streakTitle}>
            <Heatmap history={s.history} />
            <Text style={styles.caption}>Reviews per day, last 12 weeks</Text>
            {s.streakDays > 0 && longest > s.streakDays ? (
              <Text style={styles.caption}>Longest streak {longest} days</Text>
            ) : null}
            {live && liveStats ? (
              <Text style={styles.caption}>
                This week: {liveStats.this_week.reviews} {liveStats.this_week.reviews === 1 ? 'review' : 'reviews'},{' '}
                {liveStats.this_week.mined} added, {liveStats.this_week.known_added} newly known
              </Text>
            ) : null}
          </Section>

          {bands.length > 0 ? (
            <Section title="Coverage by frequency">
              <Text style={styles.caption}>How much of each band you know. Words you have never met count as unknown.</Text>
              <View style={[styles.bands, { marginTop: spacing.md }]}>
                {bands.map((b) => {
                  // Coverage against the band's size; open-ended bands fall back to tracked words.
                  // Sizes are nominal and a band can hold more dictionary words, so cap at 100%.
                  const denom = b.size || b.total;
                  const pct = Math.min(100, Math.round((b.known / Math.max(1, denom)) * 100));
                  return (
                    <View key={b.band} style={styles.bandRow}>
                      <View style={styles.bandHead}>
                        <Text style={styles.bandLabel}>{b.label}</Text>
                        <Text style={styles.caption}>
                          {b.known.toLocaleString()} of {denom.toLocaleString()}{b.size ? '' : ' met'} · {pct}%
                        </Text>
                      </View>
                      <View style={styles.milestoneTrack}>
                        <View style={[styles.milestoneBar, { width: `${pct}%` }]} />
                      </View>
                    </View>
                  );
                })}
              </View>
            </Section>
          ) : null}
        </>
      )}
    </ScrollView>
  );
}

function StatCard({
  label,
  value,
  accent,
}: {
  label: string;
  value: number;
  accent?: string;
}) {
  return (
    <View style={styles.statCard}>
      <Text style={[styles.statValue, accent ? { color: accent } : null]}>
        {value}
      </Text>
      <Text style={styles.statLabel}>{label}</Text>
    </View>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      {children}
    </View>
  );
}

/** GitHub-style activity grid: columns = weeks, rows = days. */
function Heatmap({ history }: { history: DayActivity[] }) {
  const weeks: DayActivity[][] = [];
  for (let i = 0; i < history.length; i += 7) {
    weeks.push(history.slice(i, i + 7));
  }
  const max = Math.max(1, ...history.map((d) => d.reviews));
  return (
    <View style={styles.heatmap}>
      {weeks.map((week, wi) => (
        <View key={wi} style={styles.heatCol}>
          {week.map((day) => (
            <View
              key={day.date}
              style={[styles.heatCell, { backgroundColor: heatColor(day.reviews, max) }]}
            />
          ))}
        </View>
      ))}
    </View>
  );
}

function heatColor(reviews: number, max: number): string {
  if (reviews === 0) return colors.surface2;
  const t = Math.min(1, reviews / max);
  // Accent ramp; alpha encodes intensity.
  const alpha = 0.25 + t * 0.75;
  return `rgba(${colors.accentRgb}, ${alpha.toFixed(2)})`;
}


const styles = StyleSheet.create({
  scroll: {
    paddingBottom: spacing.xl,
  },
  error: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
    fontSize: font.xs,
    color: colors.statusUnknown,
  },
  loadingWrap: {
    alignItems: 'center',
    gap: spacing.sm,
    paddingVertical: spacing.xl * 2,
  },
  loadingText: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  statGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    paddingHorizontal: spacing.lg,
  },
  statCard: {
    flexBasis: '47%',
    flexGrow: 1,
    paddingVertical: spacing.sm,
    alignItems: 'center',
  },
  statValue: {
    fontSize: font.display,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  statLabel: {
    marginTop: 2,
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  section: {
    marginTop: spacing.lg,
    marginHorizontal: spacing.lg,
    paddingTop: spacing.md,
    borderTopColor: colors.border,
    borderTopWidth: 1,
  },
  sectionTitle: {
    fontSize: font.md,
    fontWeight: '600',
    color: colors.textPrimary,
    marginBottom: spacing.md,
  },
  caption: {
    marginTop: spacing.xs,
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  gridNote: {
    marginHorizontal: spacing.lg,
    textAlign: 'center',
  },
  heatmap: {
    flexDirection: 'row',
    gap: 3,
  },
  heatCol: {
    gap: 3,
  },
  heatCell: {
    width: 14,
    height: 14,
    borderRadius: 3,
  },
  bands: {
    gap: spacing.md,
  },
  bandRow: {
    gap: spacing.xs,
  },
  bandHead: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
  },
  bandLabel: {
    fontSize: font.sm,
    color: colors.textPrimary,
  },
  milestoneTrack: {
    height: 8,
    borderRadius: 4,
    backgroundColor: colors.surface2,
    overflow: 'hidden',
  },
  milestoneBar: {
    height: '100%',
    backgroundColor: colors.accent,
    borderRadius: 4,
  },
});
