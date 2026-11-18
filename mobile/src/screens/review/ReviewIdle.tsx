import { Pressable, StyleSheet, Text, View } from 'react-native';
import { Icon } from '../../components/Icon';
import { MoonMark } from '../../components/MoonMark';
import { unlockAudio } from '../../lib/cardAudio';
import { clearLabel } from '../../data/catchUp';
import { sinceLabel, useVacation } from '../../data/vacation';
import { FirstWord } from './FirstWord';
import { WeekPulse } from '../../components/WeekPulse';
import { colors, font, spacing } from '../../theme';
import type { ReviewSession } from '../useReviewSession';
import { haptic, shared } from './shared';

/**
 * The Review tab's idle screen. Pure presentation over the session hook —
 * every fetch and count lives in useReviewSession.
 */
export function ReviewIdle({
  session,
  learnCount,
  offlineReady,
  onFindWord,
}: {
  session: ReviewSession;
  learnCount: number;
  offlineReady: boolean;
  onFindWord?: () => void;
}) {
  const {
    live, phase, dueCount, pendingSync, syncNotice, idleStats, newPending, starting, error,
    start, learnMore, catchUp,
  } = session;
  const vacation = useVacation();
  // Today's share is done but the backlog isn't: offer more, not "caught up".
  const shareDone = catchUp != null && dueCount === 0 && catchUp.backlog > 0;
  const caughtUp = dueCount === 0 && !shareDone;

  if (live && idleStats?.totals.cards === 0 && newPending === 0 && onFindWord) {
    return <FirstWord onFindWord={onFindWord} />;
  }

  if (vacation.since) {
    return (
      <View style={shared.center}>
        <View style={styles.topSpacer} />
        <MoonMark />
        <Text style={shared.dueHeadline}>On vacation</Text>
        <Text style={shared.dueSub}>Reviews paused since {sinceLabel(vacation.since)}</Text>
        {vacation.error ?? error ? <Text style={shared.error}>{vacation.error ?? error}</Text> : null}
        <Pressable
          style={({ pressed }) => [shared.cta, styles.ctaGrouped, pressed && shared.pressed]}
          onPress={() => { haptic('select'); void vacation.setOn(false); }}
          disabled={vacation.busy}
          accessibilityRole="button"
        >
          <Text style={shared.ctaLabel}>{vacation.busy ? 'Loading…' : 'End vacation'}</Text>
        </Pressable>
        {dueCount ? (
          <Pressable
            style={({ pressed }) => [shared.ctaSecondary, styles.ctaGrouped, pressed && shared.pressed]}
            onPress={() => { haptic('select'); void start(); }}
            disabled={phase === 'loading'}
            accessibilityRole="button"
          >
            <Text style={shared.ctaSecondaryLabel}>
              {phase === 'loading' ? 'Loading…' : `Review anyway · ${dueCount} due`}
            </Text>
          </Pressable>
        ) : null}
        <View style={styles.bottomSpacer} />
      </View>
    );
  }

  return (
    <View style={shared.center}>
      <View style={styles.topSpacer} />
      <MoonMark />
      {shareDone ? (
        <Text style={shared.dueHeadline}>Today’s catch-up is done</Text>
      ) : dueCount !== null && !caughtUp ? (
        <>
          <Text style={shared.heroNumber}>{dueCount}</Text>
          <Text style={shared.heroNumberLabel}>
            {catchUp ? 'to catch up today' : dueCount === 1 ? 'card due' : 'cards due'}
          </Text>
        </>
      ) : (
        <Text style={shared.dueHeadline}>{dueCount === null ? 'Review' : 'All caught up'}</Text>
      )}
      {/* Identify offline and sample data sources. */}
      {!live ? (
        <Text style={shared.dueSub}>
          {offlineReady
            ? 'Offline · using saved cards · answers sync when you reconnect'
            : 'Sample cards · explore a review before connecting your library in Settings'}
        </Text>
      ) : null}
      {catchUp ? (
        <Text style={shared.dueSub}>
          {catchUp.backlog} due in total · {clearLabel(catchUp.days_left)}
        </Text>
      ) : null}
      {pendingSync > 0 ? (
        <Text style={shared.dueSub}>
          {pendingSync} saved {pendingSync === 1 ? 'answer' : 'answers'} waiting to sync
        </Text>
      ) : null}
      {syncNotice ? <Text style={shared.dueSub}>{syncNotice}</Text> : null}
      {/* Seven-day activity and due forecast. */}
      {live ? <WeekPulse stats={idleStats} /> : null}
      {error ? <Text style={shared.error}>{error}</Text> : null}
      {!caughtUp || dueCount === null || shareDone ? (
        <Pressable
          style={({ pressed }) => [shared.cta, live && styles.ctaGrouped, pressed && shared.pressed]}
          onPress={() => { haptic('select'); unlockAudio(); void start(); }}
          disabled={phase === 'loading'}
          accessibilityRole="button"
        >
          <Text style={shared.ctaLabel}>
            {phase === 'loading' ? 'Loading…' : shareDone ? 'Keep going' : 'Start review'}
          </Text>
        </Pressable>
      ) : live && caughtUp && newPending > 0 ? (
        <Pressable
          style={({ pressed }) => [shared.cta, styles.ctaGrouped, pressed && shared.pressed]}
          onPress={() => { haptic('select'); unlockAudio(); void learnMore(); }}
          disabled={starting}
          accessibilityRole="button"
        >
          <Text style={shared.ctaLabel}>
            {starting ? 'Loading…' : `Learn ${learnCount} new ${learnCount === 1 ? 'word' : 'words'}`}
          </Text>
        </Pressable>
      ) : (
        <View style={[styles.caughtUpRow, live && styles.ctaGrouped]}>
          <Icon name="moon-full" size={16} color={colors.textTertiary} />
          <Text style={styles.caughtUpText}>Nothing due · oyasumi</Text>
        </View>
      )}
      <View style={styles.bottomSpacer} />
    </View>
  );
}

const styles = StyleSheet.create({
  ctaGrouped: {
    marginTop: spacing.sm,
  },
  topSpacer: { flex: 2 },
  bottomSpacer: { flex: 3 },
  caughtUpRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.xs,
    marginTop: spacing.lg,
  },
  caughtUpText: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
});
