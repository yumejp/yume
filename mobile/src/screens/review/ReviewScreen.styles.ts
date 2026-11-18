import { StyleSheet } from 'react-native';
import { colors, font, radius, spacing } from '../../theme';

export const styles = StyleSheet.create({
  root: {
    flex: 1,
  },
  headerRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  headerFill: {
    flex: 1,
  },
  pressed: {
    opacity: 0.8,
  },
  progressRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    marginHorizontal: spacing.lg,
    marginBottom: 0,
  },
  progressTrack: {
    flex: 1,
    height: 3,
    borderRadius: 2,
    backgroundColor: colors.surface2,
    overflow: 'hidden',
  },
  progressBar: {
    height: '100%',
    backgroundColor: colors.accent,
    borderRadius: 2,
  },
  // Shown once when the browser blocked card audio until a tap.
  soundBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    alignSelf: 'center',
    gap: spacing.xs,
    marginTop: spacing.sm,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
    borderRadius: radius.pill,
    backgroundColor: colors.accentMuted,
  },
  soundLabel: {
    fontSize: font.sm,
    fontWeight: '600',
    color: colors.accentText,
  },
  lapBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 3,
  },
  lapText: {
    fontSize: font.xs,
    fontWeight: '600',
    color: colors.statusUnknown,
    fontVariant: ['tabular-nums'],
  },
  savingDim: { opacity: 0.25 },
  savingOverlay: {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    alignItems: 'center',
    justifyContent: 'center',
    pointerEvents: 'none',
  },
  errorBanner: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: spacing.sm,
    marginHorizontal: spacing.md,
    marginBottom: spacing.sm,
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.md,
    borderRadius: radius.sm,
    borderWidth: 1,
    borderColor: colors.statusUnknown,
    backgroundColor: colors.surface1,
  },
  errorBannerText: {
    flex: 1,
    fontSize: font.sm,
    color: colors.statusUnknown,
  },
  noticeBanner: {
    borderColor: colors.border,
  },
  noticeBannerText: {
    color: colors.textSecondary,
  },
  endBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    paddingHorizontal: spacing.sm,
    paddingVertical: 9,
    marginRight: spacing.md,
  },
  endLabel: {
    fontSize: font.sm,
    color: colors.textTertiary,
    fontWeight: '600',
  },
  iconBtn: {
    alignItems: 'center',
    justifyContent: 'center',
    minWidth: 40,
    paddingVertical: 9,
    marginRight: spacing.xs,
  },
  menuBackdrop: {
    flex: 1,
    backgroundColor: colors.backdrop,
    alignItems: 'flex-end',
  },
  menuCard: {
    marginTop: 64,
    marginRight: spacing.lg,
    minWidth: 230,
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingVertical: spacing.xs,
  },
  menuRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingHorizontal: spacing.lg,
    paddingVertical: 13,
  },
  menuRowPressed: {
    backgroundColor: colors.surface2,
  },
  menuLabel: {
    fontSize: font.md,
    fontWeight: '600',
  },
  cardScroll: {
    flexGrow: 1,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  // Must fill the scroll area or the card loses its flex height.
  cardFade: {
    flex: 1,
  },
  toast: {
    position: 'absolute',
    bottom: 96,
    alignSelf: 'center',
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingVertical: 8,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface2,
  },
  gradToast: {
    borderColor: colors.statusKnown,
  },
  gradToastDemote: {
    borderColor: colors.statusLearning,
  },
  toastText: {
    color: colors.textPrimary,
    fontSize: font.sm,
    fontWeight: '600',
  },
  toastMuted: {
    color: colors.textSecondary,
    fontSize: font.sm,
  },
  toastUndo: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  toastUndoText: {
    color: colors.accentText,
    fontSize: font.sm,
    fontWeight: '700',
  },
  ratings: {
    flexDirection: 'row',
    gap: spacing.xs,
    marginHorizontal: spacing.md,
    paddingTop: spacing.sm,
    paddingBottom: spacing.md,
    borderTopWidth: 1,
    borderTopColor: colors.border,
  },
  ratingBtn: {
    flex: 1,
    borderRadius: radius.md,
    paddingVertical: 10,
    alignItems: 'center',
    gap: 2,
  },
  ratingBtnPressed: {
    backgroundColor: colors.accentMuted,
  },
  ratingBtnDimmed: {
    opacity: 0.4,
  },
  ratingLabel: {
    fontSize: font.sm,
    fontWeight: '600',
    color: colors.textPrimary,
    textAlign: 'center',
  },
  ratingLabelAgain: {
    color: colors.textSecondary,
  },
  // Keep readable: 11px tertiary gray is below AA on OLED black.
  ratingWhen: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  ratingKey: {
    marginTop: 2,
    fontSize: 9,
    color: colors.textTertiary,
    opacity: 0.7,
  },
  revealBtn: {
    flex: 1,
    backgroundColor: colors.accentMuted,
    borderRadius: radius.md,
    paddingVertical: 14,
  },
  revealLabel: {
    color: colors.accentText,
    fontSize: font.md,
    fontWeight: '600',
    textAlign: 'center',
  },
});

