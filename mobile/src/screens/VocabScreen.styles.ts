import { StyleSheet } from 'react-native';
import { colors, font, fontJa, radius, spacing } from '../theme';

export const styles = StyleSheet.create({
  root: {
    flex: 1,
  },
  controls: {
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.sm,
    gap: spacing.sm,
    paddingBottom: spacing.sm,
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
  searchRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
  },
  searchFill: {
    flex: 1,
  },
  editBtn: {
    alignItems: 'center',
    justifyContent: 'center',
    width: 42,
    height: 42,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface1,
  },
  editBtnActive: {
    borderColor: colors.accentText,
    backgroundColor: colors.accentMuted,
  },
  filterRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: 6,
  },
  bandRow: {
    flexWrap: 'nowrap',
  },
  bandChip: {
    paddingHorizontal: 8,
    flexShrink: 1,
  },
  chip: {
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: 10,
    paddingVertical: 5,
    backgroundColor: colors.surface1,
  },
  chipLabel: {
    fontSize: font.xs,
    color: colors.textSecondary,
  },
  searchFocused: {
    borderColor: colors.accentText,
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
    height: 1,
    backgroundColor: colors.border,
  },
  band: {
    paddingTop: spacing.md,
    paddingBottom: spacing.sm,
    gap: spacing.sm,
  },
  bandHead: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
    gap: spacing.sm,
  },
  bandLabel: {
    fontSize: font.md,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  bandMeta: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  tiles: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
  },
  tile: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingVertical: 7,
    paddingHorizontal: 10,
    borderRadius: radius.md,
    backgroundColor: colors.surface1,
    borderWidth: 1,
    borderColor: colors.border,
  },
  tileDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  tileText: {
    fontFamily: fontJa,
    fontSize: font.jp,
    color: colors.textPrimary,
  },
  tileTextKnown: {
    color: colors.textSecondary,
  },
  empty: {
    marginTop: spacing.xl,
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.sm,
  },
  // Labels rows that are not live: sample words or an offline copy.
  notice: {
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    borderRadius: radius.md,
    backgroundColor: colors.surface2,
    color: colors.textSecondary,
    fontSize: font.sm,
  },
  footerMore: {
    paddingVertical: spacing.md,
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.xs,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
  },
  rowTap: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: spacing.md,
    gap: spacing.md,
  },
  rowPressed: {
    opacity: 0.6,
  },
  rowDot: {
    width: 12,
    height: 12,
    borderRadius: 6,
    borderWidth: 2,
  },
  rowMain: {
    flex: 1,
    gap: 3,
  },
  rowHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: spacing.sm,
  },
  lemma: {
    fontFamily: fontJa,
    fontSize: font.lg,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  reading: {
    fontFamily: fontJa,
    fontSize: font.sm,
    color: colors.accentText,
  },
  gloss: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  glossEmpty: {
    fontSize: font.sm,
    color: colors.textTertiary,
    fontStyle: 'italic',
  },
  tagRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
    marginTop: 2,
  },
  tag: {
    fontSize: font.xs,
    color: colors.textTertiary,
    backgroundColor: colors.surface2,
    borderRadius: radius.sm,
    paddingHorizontal: 6,
    paddingVertical: 2,
    overflow: 'hidden',
  },
  delete: {
    padding: spacing.xs,
    marginLeft: spacing.xs,
  },
});

