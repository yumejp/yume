import { StyleSheet } from 'react-native';
import { colors, font, fontJa, radius, spacing } from '../theme';

export const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'flex-end',
    // Without this a long sentence sizes to its content and overflows the card.
    maxWidth: '100%',
  },
  trailing: {
    marginLeft: spacing.xs,
    alignSelf: 'flex-end',
  },
  unit: {
    flexDirection: 'row',
    alignItems: 'flex-end',
  },
  col: {
    alignItems: 'center',
  },
  colTarget: {
    backgroundColor: colors.accentMuted,
    borderRadius: radius.sm,
    paddingHorizontal: 3,
  },
  furi: {
    fontFamily: fontJa,
    color: colors.textSecondary,
    textAlign: 'center',
  },
  backdrop: {
    flex: 1,
    backgroundColor: colors.backdrop,
    justifyContent: 'center',
    padding: spacing.lg,
  },
  sheet: {
    backgroundColor: colors.surface2,
    borderRadius: radius.lg,
    borderColor: colors.border,
    borderWidth: 1,
    padding: spacing.lg,
    maxHeight: '70%',
  },
  headRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: spacing.sm,
  },
  headword: {
    fontFamily: fontJa,
    fontSize: font.xl,
    color: colors.textPrimary,
  },
  headReading: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.textSecondary,
    marginTop: 2,
  },
  chips: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    marginTop: spacing.md,
  },
  chip: {
    borderWidth: 1.5,
    borderRadius: radius.md,
    paddingVertical: spacing.xs,
    paddingHorizontal: spacing.md,
  },
  chipLabel: {
    fontSize: font.sm,
    fontWeight: '600',
  },
  body: {
    minHeight: 150,
  },
  loadingWrap: {
    minHeight: 150,
    alignItems: 'center',
    justifyContent: 'center',
  },
  dim: {
    fontSize: font.sm,
    color: colors.textTertiary,
    marginTop: spacing.md,
  },
  entry: {
    marginTop: spacing.md,
    paddingTop: spacing.md,
    borderTopColor: colors.border,
    borderTopWidth: 1,
  },
  entryHead: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.textPrimary,
    marginBottom: spacing.xs,
  },
  sense: {
    marginTop: spacing.xs,
  },
  pos: {
    fontSize: font.xs,
    color: colors.accentText,
    fontStyle: 'italic',
  },
  gloss: {
    fontSize: font.sm,
    color: colors.textPrimary,
  },
  close: {
    marginTop: spacing.lg,
    alignSelf: 'flex-end',
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.md,
    backgroundColor: colors.accentMuted,
  },
  closeLabel: {
    color: colors.accentText,
    fontSize: font.sm,
    fontWeight: '600',
  },
});

