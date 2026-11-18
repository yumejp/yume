import { Pressable, StyleSheet, Text, View } from 'react-native';
import { colors, font, radius, spacing } from '../theme';

/**
 * iOS-style segmented control for swapping between sibling sub-views inside
 * a single tab (e.g. Library's Words / Stats). Generic over the segment key.
 */
export function SegmentedControl<T extends string>({
  segments,
  active,
  onChange,
}: {
  segments: { key: T; label: string }[];
  active: T;
  onChange: (key: T) => void;
}) {
  return (
    <View style={styles.track}>
      {segments.map((s) => {
        const isActive = s.key === active;
        return (
          <Pressable
            key={s.key}
            style={[styles.segment, isActive && styles.segmentActive]}
            onPress={() => onChange(s.key)}
            accessibilityRole="button"
            accessibilityState={{ selected: isActive }}
          >
            <Text style={[styles.label, isActive && styles.labelActive]}>{s.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  track: {
    flexDirection: 'row',
    backgroundColor: colors.surface2,
    borderRadius: radius.md,
    padding: 3,
    marginHorizontal: spacing.lg,
    marginTop: spacing.sm,
    gap: 3,
  },
  segment: {
    flex: 1,
    alignItems: 'center',
    paddingVertical: 7,
    borderRadius: radius.sm,
  },
  // Accent-tinted thumb: a darker-than-track active state read as "off" on
  // OLED; this matches the app's other segmented rows (Dictionary, Settings).
  segmentActive: {
    backgroundColor: colors.accentMuted,
  },
  label: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  labelActive: {
    color: colors.accentText,
    fontWeight: '600',
  },
});
