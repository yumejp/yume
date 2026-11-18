import { Pressable, Text, View } from 'react-native';
import { Icon } from '../components/Icon';
import type { Card } from '../data/types';
import { useTextScale } from '../settings/SettingsContext';
import { colors, font, langJa, statusColor } from '../theme';
import { styles } from './VocabScreen.styles';

// The furigana tag reads as what it means for the word.
const visibleTags = (tags: string[]) =>
  tags.filter((tag) => !/import/i.test(tag)).map((tag) => (tag === 'furigana' ? 'known by sound' : tag));

export function VocabRow({
  card,
  onPress,
  onDelete,
  deleting,
}: {
  card: Card;
  onPress: () => void;
  onDelete?: () => void;
  deleting?: boolean;
}) {
  const tags = visibleTags(card.tags);
  const jpScale = useTextScale();

  return (
    <View style={styles.row}>
      <Pressable
        style={({ pressed }) => [styles.rowTap, pressed && styles.rowPressed]}
        onPress={onPress}
        accessibilityRole="button"
        accessibilityLabel={`${card.lemma}${card.reading ? `, ${card.reading}` : ''}, ${card.status}`}
        accessibilityHint="Look up this word"
      >
        <View
          style={[
            styles.rowDot,
            { backgroundColor: statusColor(card.status), borderColor: statusColor(card.status) + '44' },
          ]}
        />
        <View style={styles.rowMain}>
          <View style={styles.rowHead}>
            <Text {...langJa} style={[styles.lemma, jpScale !== 1 && { fontSize: Math.round(font.lg * jpScale) }]}>
              {card.lemma}
            </Text>
            {card.reading ? (
              <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(font.sm * jpScale) }]}>
                {card.reading}
              </Text>
            ) : null}
          </View>
          {card.glosses.length > 0 ? (
            <Text style={styles.gloss} numberOfLines={1}>
              {card.glosses.join(' · ')}
            </Text>
          ) : (
            <Text style={styles.glossEmpty} numberOfLines={1}>no definition yet</Text>
          )}
          {tags.length > 0 ? (
            <View style={styles.tagRow}>
              {tags.map((tag) => <Text key={tag} style={styles.tag}>{tag}</Text>)}
            </View>
          ) : null}
        </View>
      </Pressable>
      {onDelete ? (
        <Pressable
          style={({ pressed }) => [styles.delete, pressed && styles.rowPressed]}
          onPress={onDelete}
          disabled={deleting}
          hitSlop={8}
          accessibilityRole="button"
          accessibilityLabel={`Delete the ${card.lemma} card`}
        >
          <Icon
            name="trash-can-outline"
            size={16}
            color={deleting ? colors.textTertiary : colors.statusUnknown}
          />
        </Pressable>
      ) : null}
    </View>
  );
}
