import { StyleSheet, Text, View } from 'react-native';
import type { YumeClient } from '../api/client';
import { useTextScale } from '../settings/SettingsContext';
import { JapaneseSentence } from './JapaneseSentence';
import { SpeakButton } from './SpeakButton';
import { colors, font, fontJa, langJa, spacing } from '../theme';

interface Props {
  sentence: string;
  translation?: string;
  /** Tokenized, tap-to-define rendering needs a live client; falls back to
   *  plain text (sample mode / offline). */
  client: YumeClient | null;
  /** Lemma to tint inside the sentence (the card's word). */
  highlight?: string;
  /** Active-recall: keep the translation hidden until revealed. */
  hideTranslation?: boolean;
  /** Device-voice speaker after the sentence; off when a generated clip exists. */
  speaker?: boolean;
}

/** A card's example sentence: tokenized Japanese + Listen button, with the
 *  translation underneath. Shared by the mined `card.sentence` and the
 *  📝-note sentence so the two render identically. */
export function SentenceBlock({ sentence, translation, client, highlight, hideTranslation, speaker = true }: Props) {
  // JapaneseSentence scales itself; the plain fallback must match by hand.
  const scale = useTextScale();
  return (
    <View style={styles.block}>
      {client ? (
        <JapaneseSentence
          text={sentence}
          client={client}
          size={font.xl}
          highlight={highlight}
          trailing={speaker ? <SpeakButton text={sentence} lang="ja-JP" compact /> : undefined}
        />
      ) : (
        <View style={styles.row}>
          <Text {...langJa} style={[styles.plain, { fontSize: Math.round(font.xl * scale) }]}>{sentence}</Text>
          {speaker ? <SpeakButton text={sentence} lang="ja-JP" compact /> : null}
        </View>
      )}
      {!hideTranslation && translation ? (
        <Text style={styles.translation}>{translation}</Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  block: {
    marginTop: spacing.sm,
    paddingTop: spacing.sm,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    alignSelf: 'stretch',
    alignItems: 'flex-start',
    gap: spacing.sm,
  },
  row: {
    alignSelf: 'stretch',
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: spacing.xs,
  },
  plain: {
    fontFamily: fontJa,
    fontSize: font.xl,
    color: colors.textPrimary,
  },
  translation: {
    fontSize: font.md,
    lineHeight: font.md + 7,
    color: colors.textSecondary,
  },
});
