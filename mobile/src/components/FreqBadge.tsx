import { StyleSheet, Text, View } from 'react-native';
import { colors, font, radius } from '../theme';

/** Upper rank of each Library frequency band, labelled the same way. */
const BANDS = [1000, 3000, 6000, 12000];

/**
 * "How common is this word" chip for dictionary surfaces — the add-it-or-not
 * signal when a word turns up in a wild sentence. Fed by DictEntry.priority
 * (JMdict ke_pri/re_pri): 1–48 = newspaper frequency band (band × 500 ≈ rank,
 * so nf10 ≈ the top 5,000 words), 60 = the ichi1/news1 common classes,
 * 80 = the *2 classes. Missing priority is unknown, not evidence of rarity.
 * `common` (ichi1/news1 on some form) wins over the nf band: 食べる is ichi1
 * but only nf25, which read as "top 13k". Ranks are rounded up to the
 * Library's bands, never to the nearest thousand.
 */
export function freqInfo(
  priority: number | undefined,
  common?: boolean,
): { label: string; tint: string } | null {
  if (common) return { label: 'common', tint: colors.statusKnown };
  if (priority == null || !Number.isInteger(priority) || priority < 1) return null;
  if (priority <= 48) {
    const rank = priority * 500;
    const band = BANDS.find((max) => rank <= max);
    const label = band ? `top ${band / 1000}k` : `${BANDS[BANDS.length - 1] / 1000}k+`;
    return { label, tint: rank <= 6000 ? colors.statusKnown : colors.statusLearning };
  }
  if (priority === 60) return { label: 'common', tint: colors.statusKnown };
  return priority === 80 ? { label: 'less common', tint: colors.statusLearning } : null;
}

export function FreqBadge({ priority, common }: { priority: number | undefined; common?: boolean }) {
  const info = freqInfo(priority, common);
  if (!info) return null;
  const { label, tint } = info;
  return (
    <View style={[styles.badge, { borderColor: tint + '55' }]}>
      <Text style={[styles.label, { color: tint }]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: {
    borderWidth: 1,
    borderRadius: radius.sm,
    paddingHorizontal: 8,
    paddingVertical: 2,
    alignSelf: 'flex-start',
  },
  label: {
    fontSize: font.xs,
    fontWeight: '600',
  },
});
