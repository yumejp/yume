import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { Icon } from '../../components/Icon';
import { MoonMark } from '../../components/MoonMark';
import { unlockAudio } from '../../lib/cardAudio';
import { colors, font, spacing } from '../../theme';
import type { ReviewSession } from '../useReviewSession';
import { haptic, shared } from './shared';

/**
 * The session-complete screen. Everything below the headline waits on
 * finishSession's refetches (doneReady) and renders inside one fixed-footprint
 * slot so the headline doesn't move when counts land.
 */
export function ReviewDone({
  session,
  learnCount,
}: {
  session: ReviewSession;
  learnCount: number;
}) {
  const {
    live, mode, doneReady, moreDue, newPending, doneCount, againCount, pendingSync, error,
    starting, canUndo, submitting, start, learnMore, endSession, undo, catchUp,
  } = session;
  // moreDue is today's share during catch-up; past it, offer the rest of the backlog.
  const shareDone = catchUp != null && moreDue === 0 && catchUp.backlog > 0;
  const left = shareDone ? catchUp.backlog : moreDue;
  const keepGoing = doneReady && live && left > 0;
  // New words wait until the backlog is cleared.
  const canLearnNew = doneReady && live && newPending > 0 && catchUp == null;
  const hasMore = keepGoing || canLearnNew;
  const plural = (n: number, one: string) => `${n} ${n === 1 ? one : `${one}s`}`;

  // againCount counts RATINGS, not cards.
  const subline = [
    mode === 'sample' ? 'Sample cards · progress isn’t saved' : null,
    mode === 'offline' && pendingSync > 0 ? `${plural(pendingSync, 'answer')} waiting to sync` : null,
    againCount > 0 ? `${againCount} marked Again` : null,
  ].filter(Boolean).join(' · ');

  return (
    <View style={shared.center}>
      <View style={styles.topSpacer} />
      <MoonMark />
      <Text style={shared.heroNumber}>{doneCount}</Text>
      <Text style={shared.heroNumberLabel}>{doneCount === 1 ? 'card reviewed' : 'cards reviewed'}</Text>
      {subline ? <Text style={shared.dueSub}>{subline}</Text> : null}
      {error ? <Text style={shared.error}>{error}</Text> : null}
      <View style={styles.followUp}>
        {!doneReady || starting ? (
          <View style={styles.ctaPlaceholder}>
            <ActivityIndicator size="small" color={colors.textTertiary} />
          </View>
        ) : (
          <>
            {live && !hasMore ? (
              <View style={styles.caughtUpRow}>
                <Icon name="moon-full" size={16} color={colors.textTertiary} />
                <Text style={styles.ghostLabel}>Nothing left tonight · oyasumi</Text>
              </View>
            ) : null}
            {shareDone && doneReady ? (
              <Text style={styles.ghostLabel}>Today’s catch-up is done</Text>
            ) : null}
            {keepGoing ? (
              <Pressable
                style={({ pressed }) => [shared.cta, pressed && shared.pressed]}
                onPress={() => { haptic('select'); unlockAudio(); void start(); }}
                accessibilityRole="button"
              >
                <Text style={shared.ctaLabel}>Keep going · {left} left</Text>
              </Pressable>
            ) : null}
            {canLearnNew ? (
              <Pressable
                style={({ pressed }) => [
                  keepGoing ? shared.ctaSecondary : shared.cta,
                  pressed && shared.pressed,
                ]}
                onPress={() => { haptic('select'); unlockAudio(); void learnMore(); }}
                accessibilityRole="button"
              >
                <Text style={keepGoing ? shared.ctaSecondaryLabel : shared.ctaLabel}>
                  <Icon name="sprout" size={14} /> Learn {plural(learnCount, 'new word')}
                </Text>
              </Pressable>
            ) : null}
            {!hasMore ? (
              <Pressable
                style={({ pressed }) => [shared.cta, pressed && shared.pressed]}
                onPress={endSession}
                accessibilityRole="button"
              >
                <Text style={shared.ctaLabel}>Back to start</Text>
              </Pressable>
            ) : null}
            <View style={styles.ghostRow}>
              {hasMore ? (
                <Pressable
                  style={({ pressed }) => [styles.ghost, pressed && shared.pressed]}
                  onPress={endSession}
                  accessibilityRole="button"
                >
                  <Text style={styles.ghostLabel}>End study</Text>
                </Pressable>
              ) : null}
              {hasMore && canUndo ? <Text style={styles.ghostLabel}>·</Text> : null}
              {canUndo ? (
                <Pressable
                  style={({ pressed }) => [styles.ghost, pressed && shared.pressed]}
                  onPress={() => { haptic('tap'); void undo(); }}
                  disabled={submitting || starting}
                  accessibilityRole="button"
                >
                  <Icon name="undo-variant" size={14} color={colors.textTertiary} />
                  <Text style={styles.ghostLabel}>Undo last rating</Text>
                </Pressable>
              ) : null}
            </View>
          </>
        )}
      </View>
      <View style={styles.bottomSpacer} />
    </View>
  );
}

const styles = StyleSheet.create({
  // Same 2:3 bias as the idle screen: the done screen swaps in for it in place.
  topSpacer: { flex: 2 },
  bottomSpacer: { flex: 3 },
  // Tallest arrangement: primary + secondary CTA + ghost row.
  followUp: {
    minHeight: 168,
    alignSelf: 'stretch', // shrink-wrap would re-center on every content change
    alignItems: 'center',
  },
  caughtUpRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.xs,
    marginTop: spacing.md,
  },
  // Sized like the primary CTA it stands in for.
  ctaPlaceholder: {
    marginTop: spacing.lg,
    height: 48,
    justifyContent: 'center',
  },
  ghostRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    marginTop: spacing.lg,
  },
  ghost: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  ghostLabel: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
});
