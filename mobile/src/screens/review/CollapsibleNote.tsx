import { useEffect, useState, type ReactNode } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { Icon, type IconName } from '../../components/Icon';
import { colors, font, spacing } from '../../theme';

/** A titled note that starts open the first time it's seen, then stays collapsed
 *  on later cards (same behavior the tutor note has always had). */
export function CollapsibleNote({
  title,
  icon,
  body,
  startExpanded,
  onSeen,
  content,
}: {
  title: string;
  icon?: IconName;
  body: string;
  startExpanded: boolean;
  onSeen: () => void;
  /** Replaces the plain body text when expanded (body still gates rendering). */
  content?: ReactNode;
}) {
  const [expanded, setExpanded] = useState(startExpanded);
  useEffect(() => {
    if (expanded) onSeen();
  }, [expanded, onSeen]);
  if (!body) return null;
  return (
    <View style={styles.note}>
      <Pressable
        style={styles.noteHeader}
        onPress={() => setExpanded((e) => !e)}
        accessibilityRole="button"
        accessibilityState={{ expanded }}
      >
        <View style={styles.noteTitleRow}>
          {icon ? <Icon name={icon} size={14} color={colors.textSecondary} /> : null}
          <Text style={styles.noteTitle}>{title}</Text>
        </View>
        <Icon
          name={expanded ? 'chevron-up' : 'chevron-down'}
          size={16}
          color={colors.textTertiary}
        />
      </Pressable>
      {expanded ? (content ?? <Text style={styles.noteText}>{body}</Text>) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  note: {
    borderTopColor: colors.border,
    borderTopWidth: 1,
    paddingTop: spacing.sm,
  },
  noteHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    minHeight: 32,
  },
  noteTitleRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  noteTitle: {
    fontSize: font.sm,
    fontWeight: '600',
    color: colors.textSecondary,
  },
  noteText: {
    fontSize: font.md,
    color: colors.textPrimary,
    lineHeight: font.md + 8,
    marginTop: spacing.xs,
  },
});
