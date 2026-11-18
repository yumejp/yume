import { StyleSheet } from 'react-native';
import { colors, font, fontDisplay, fontJa, radius, spacing } from '../../theme';

export const styles = StyleSheet.create({
  // No box: the card is the page, the rating row below is its edge.
  card: {
    flex: 1,
    padding: spacing.md,
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: 320,
  },
  cardTop: {
    justifyContent: 'flex-start',
    paddingTop: spacing.lg,
  },
  lemma: {
    fontFamily: fontDisplay,
    fontSize: 50,
    fontWeight: '600',
    color: colors.textPrimary,
    textAlign: 'center',
  },
  lemmaFront: {
    fontFamily: fontDisplay,
    fontSize: 92,
    fontWeight: '600',
    color: colors.textPrimary,
    textAlign: 'center',
  },
  // Absolute so the lemma sits at the true center of the face.
  clozeFront: {
    fontFamily: fontJa,
    fontSize: 28,
    lineHeight: 44,
    color: colors.textPrimary,
    textAlign: 'center',
    paddingHorizontal: spacing.lg,
  },
  clozeBlank: {
    color: colors.accentText,
    fontWeight: '700',
  },
  rubyRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    justifyContent: 'center',
    alignItems: 'flex-end',
  },
  rubyCol: {
    alignItems: 'center',
  },
  rubyText: {
    fontFamily: fontJa,
    color: colors.textSecondary,
  },
  flipHint: {
    position: 'absolute',
    bottom: spacing.lg,
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  answer: {
    marginTop: spacing.md,
    alignItems: 'center',
    gap: spacing.sm,
  },
  sourceImage: {
    aspectRatio: 16 / 9,
    borderRadius: radius.md,
    backgroundColor: colors.surface2,
  },
  reading: {
    fontFamily: fontJa,
    fontSize: 20,
    color: colors.textSecondary,
  },
  splitRow: {
    flexDirection: 'row',
    justifyContent: 'center',
    gap: spacing.sm,
  },
  splitCol: {
    alignItems: 'center',
  },
  splitKanji: {
    fontFamily: fontJa,
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  // Japanese inside an English line ("also written …", a Japanese definition).
  ja: {
    fontFamily: fontJa,
  },
  alsoWritten: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  audioBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    minHeight: 40,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
    borderRadius: radius.sm,
    backgroundColor: colors.accentMuted,
  },
  audioBtnLabel: {
    fontSize: font.sm,
    color: colors.accentText,
    fontWeight: '600',
  },
  clipRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: spacing.sm,
  },
  glosses: {
    fontSize: 19,
    color: colors.textPrimary,
    textAlign: 'center',
  },
  glossesSecondary: {
    fontSize: font.md,
    color: colors.textSecondary,
    textAlign: 'center',
  },
  translation: {
    fontSize: font.md,
    lineHeight: font.md + 7,
    color: colors.textSecondary,
  },
  showEnglish: {
    fontSize: font.sm,
    fontWeight: '600',
    color: colors.accentText,
    paddingVertical: spacing.xs,
  },
  notesBlock: {
    marginTop: spacing.sm,
    alignSelf: 'stretch',
    gap: spacing.sm,
  },
  teach: {
    gap: spacing.xs,
  },
  furiganaRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: spacing.md,
  },
  furiganaNote: {
    flexShrink: 1,
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  teachLinks: {
    flexDirection: 'row',
    gap: spacing.lg,
  },
  teachLink: {
    fontSize: font.sm,
    fontWeight: '600',
    color: colors.accentText,
    paddingVertical: spacing.xs,
  },
  teachDot: {
    color: colors.accent,
  },
  teachText: {
    fontSize: font.md,
    lineHeight: font.md + 8,
    color: colors.textPrimary,
  },
  mnemonic: {
    marginTop: spacing.xs,
    gap: spacing.sm,
  },
  breakdown: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.md,
    rowGap: spacing.sm,
  },
  breakdownPart: {
    alignItems: 'center',
    gap: 2,
    maxWidth: 140,
  },
  // A part the learner has not met in any Known word: worth a tap.
  breakdownNew: { position: 'absolute', top: 6, right: 6, width: 6, height: 6, borderRadius: 3, backgroundColor: colors.accent },
  breakdownKanji: {
    fontFamily: fontDisplay,
    fontSize: 26,
    lineHeight: 32,
    color: colors.textPrimary,
  },
  breakdownMeaning: {
    fontSize: font.xs,
    color: colors.textSecondary,
    textAlign: 'center',
  },
  story: {
    fontSize: font.md,
    lineHeight: font.md + 8,
    color: colors.textPrimary,
  },
  voiceNote: {
    fontSize: font.xs,
    color: colors.textTertiary,
    textAlign: 'center',
    marginTop: spacing.xs,
  },
  examplesBlock: {
    marginTop: spacing.sm,
    paddingTop: spacing.sm,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    alignSelf: 'stretch',
    gap: spacing.sm,
  },
  examplesHeading: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  exampleItem: {
    gap: 2,
  },
  exampleHead: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: spacing.sm,
  },
  exampleSentence: {
    flex: 1,
  },
  exampleText: {
    fontFamily: fontJa,
    fontSize: 18,
    color: colors.textPrimary,
  },
  exampleTranslation: {
    fontSize: font.md,
    color: colors.textSecondary,
  },
  examplesCredit: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  newBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    alignSelf: 'center',
    gap: spacing.xs,
    marginBottom: spacing.md,
    paddingVertical: 5,
    paddingHorizontal: spacing.md,
    borderRadius: radius.md,
    backgroundColor: colors.accentMuted,
  },
  newBadgeText: {
    color: colors.accentText,
    fontSize: font.xs,
    fontWeight: '700',
  },
});

