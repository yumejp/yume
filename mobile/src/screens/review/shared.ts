import * as Haptics from 'expo-haptics';
import { StyleSheet } from 'react-native';
import { colors, font, fontDisplay, radius, spacing } from '../../theme';

/** Fire-and-forget haptic — a no-op wherever unsupported (web). */
export function haptic(kind: 'tap' | 'select' | 'success') {
  const p =
    kind === 'tap'
      ? Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)
      : kind === 'select'
        ? Haptics.selectionAsync()
        : Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
  p.catch(() => {});
}

/** Layout + typography shared by the idle and done "poster" screens. */
export const shared = StyleSheet.create({
  center: {
    flex: 1,
    alignItems: 'center',
    padding: spacing.lg,
  },
  dueHeadline: {
    fontFamily: fontDisplay,
    fontSize: font.xl,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  heroNumber: {
    fontFamily: fontDisplay,
    fontSize: font.display + 8,
    lineHeight: font.display + 14,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  heroNumberLabel: {
    fontSize: font.md,
    color: colors.textSecondary,
  },
  dueSub: {
    marginTop: spacing.xs,
    fontSize: font.sm,
    color: colors.textTertiary,
    textAlign: 'center',
  },
  error: {
    marginTop: spacing.sm,
    fontSize: font.sm,
    color: colors.statusUnknown,
    textAlign: 'center',
  },
  cta: {
    marginTop: spacing.lg,
    backgroundColor: colors.accent,
    borderRadius: radius.pill,
    paddingVertical: 14,
    paddingHorizontal: spacing.xl,
    minWidth: 220,
  },
  pressed: {
    opacity: 0.8,
  },
  ctaLabel: {
    color: colors.onAccent,
    fontSize: font.md,
    fontWeight: '600',
    textAlign: 'center',
  },
  ctaSecondary: {
    marginTop: spacing.sm,
    borderRadius: radius.pill,
    minWidth: 220,
    borderWidth: 1,
    borderColor: colors.border,
    paddingVertical: 12,
    paddingHorizontal: spacing.xl,
  },
  ctaSecondaryLabel: {
    color: colors.textSecondary,
    fontSize: font.md,
    fontWeight: '600',
    textAlign: 'center',
  },
});
