import { StyleSheet } from 'react-native';
import { colors, font, fontJa, radius, spacing } from '../theme';

export const styles = StyleSheet.create({
  handoff: { marginHorizontal: spacing.lg, marginBottom: spacing.md, paddingVertical: spacing.sm,
    borderBottomWidth: 1, borderBottomColor: colors.border, flexDirection: 'row', alignItems: 'center', gap: spacing.sm, flexWrap: 'wrap' },
  handoffText: { flex: 1, color: colors.textPrimary, fontSize: font.sm },
  searchIntro: { marginTop: spacing.xl, gap: spacing.sm },
  notice: { marginHorizontal: spacing.lg, marginBottom: spacing.sm },
  root: {
    flex: 1,
  },
  controls: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
    gap: spacing.sm,
  },
  modeRow: {
    flexDirection: 'row',
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.sm,
    overflow: 'hidden',
    alignSelf: 'flex-start',
  },
  modeSeg: {
    paddingVertical: 6,
    paddingHorizontal: spacing.md,
  },
  modeSegActive: {
    backgroundColor: colors.accentMuted,
  },
  modeLabel: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  modeLabelActive: {
    color: colors.accentText,
    fontWeight: '600',
  },
  search: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: font.md,
  },
  searchDisabled: {
    opacity: 0.5,
  },
  kanaHint: {
    marginTop: spacing.xs,
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  kanaHintValue: {
    color: colors.accentText,
    fontWeight: '600',
  },
  error: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.xs,
    fontSize: font.xs,
    color: colors.statusUnknown,
  },
  list: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.lg,
    flexGrow: 1,
  },
  separator: {
    height: spacing.sm,
  },
  sectionLabel: {
    marginTop: spacing.sm,
    marginBottom: spacing.sm,
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  empty: {
    marginTop: spacing.xl,
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.sm,
    paddingHorizontal: spacing.lg,
  },
  card: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.sm,
  },
  cardHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    justifyContent: 'space-between',
  },
  headLeft: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: spacing.sm,
    flexShrink: 1,
  },
  head: {
    fontFamily: fontJa,
    fontSize: font.xl,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  readings: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.accentText,
  },
  sense: {
    gap: 3,
    marginTop: spacing.xs,
  },
  pos: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  // Glosses are the dictionary's primary content — full-strength text, not a
  // dimmed caption (the review card renders its glosses primary too).
  gloss: {
    fontSize: font.md,
    color: colors.textPrimary,
  },
  senseNum: {
    color: colors.textTertiary,
  },
  moreSenses: {
    fontSize: font.sm,
    color: colors.accentText,
    fontWeight: '600',
    marginTop: spacing.xs,
  },
  actions: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    marginTop: spacing.xs,
  },
  actionBtn: {
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
    backgroundColor: colors.surface2,
  },
  actionBtnDone: {
    backgroundColor: colors.accentMuted,
    borderColor: colors.accentText,
  },
  actionText: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  actionTextDone: {
    color: colors.textPrimary,
    fontWeight: '600',
  },
  examples: {
    gap: spacing.sm,
    marginTop: spacing.xs,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    paddingTop: spacing.sm,
  },
  example: {
    gap: 2,
  },
  exampleJa: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.textPrimary,
  },
  exampleEn: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  caption: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  // Data credit under the results and under corpus examples.
  credit: {
    fontSize: font.xs,
    color: colors.textTertiary,
    textAlign: 'center',
    marginTop: spacing.md,
  },
  exampleCredit: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
})

