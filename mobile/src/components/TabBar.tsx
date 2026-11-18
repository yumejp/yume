import { useEffect } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { gatedDueCount } from '../data/live';
import { invalidateIfStale, STATS_STALE_MS, useServerQuery } from '../data/queryStore';
import { useServer } from '../server/ServerContext';
import { useSettings } from '../settings/SettingsContext';
import { colors, font, radius, spacing } from '../theme';
import { Icon, type IconName } from './Icon';

export type TabKey = 'review' | 'library' | 'dictionary' | 'settings';

const TABS: { key: TabKey; label: string; icon: IconName }[] = [
  { key: 'review', label: 'Review', icon: 'cards-outline' },
  { key: 'library', label: 'Library', icon: 'book-open-page-variant-outline' },
  { key: 'dictionary', label: 'Dictionary', icon: 'book-search-outline' },
  { key: 'settings', label: 'Settings', icon: 'cog-outline' },
];

export function TabBar({
  active,
  onChange,
}: {
  active: TabKey;
  onChange: (tab: TabKey) => void;
}) {
  const insets = useSafeAreaInsets();
  const server = useServer();
  const { settings } = useSettings();
  const live = server.live;
  // Gated like the Review idle screen: the badge must promise the same number
  // of cards a session actually delivers.
  const statsQuery = useServerQuery('stats', live ? () => server.client!.deckStats() : null, [live]);
  // No badge on vacation: nothing is asked of the learner.
  const due = statsQuery.data && !statsQuery.data.vacation_since ? gatedDueCount(statsQuery.data) : 0;

  // Heartbeat: 'stats' drifts with time; invalidateIfStale no-ops inside the window.
  useEffect(() => {
    const t = setInterval(() => invalidateIfStale(STATS_STALE_MS, 'stats'), 60_000);
    return () => clearInterval(t);
  }, []);

  return (
    <View style={[styles.bar, { paddingBottom: Math.max(insets.bottom, spacing.sm) }]}>
      {TABS.map((t) => {
        const isActive = t.key === active;
        const tint = isActive ? colors.accent : colors.textTertiary;
        return (
          <Pressable
            key={t.key}
            style={({ pressed }) => [styles.tab, pressed && styles.tabPressed]}
            onPress={() => onChange(t.key)}
            accessibilityRole="tab"
            accessibilityState={{ selected: isActive }}
          >
            <View>
              <Icon name={t.icon} size={22} color={tint} />
              {t.key === 'review' && due > 0 ? (
                <View style={styles.badge}>
                  <Text style={styles.badgeText}>{due > 99 ? '99+' : due}</Text>
                </View>
              ) : null}
            </View>
            <Text style={[styles.label, { color: tint }]}>{t.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    flexDirection: 'row',
    backgroundColor: colors.surface1,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    paddingTop: spacing.sm,
  },
  tab: {
    flex: 1,
    alignItems: 'center',
    gap: 3,
    paddingVertical: spacing.xs,
  },
  tabPressed: {
    opacity: 0.6,
  },
  // Constant weight so labels don't shift width when the selection moves.
  label: {
    fontSize: font.xs,
    fontWeight: '600',
  },
  badge: {
    position: 'absolute',
    top: -4,
    right: -12,
    minWidth: 16,
    paddingHorizontal: 4,
    paddingVertical: 1,
    borderRadius: radius.pill,
    backgroundColor: colors.accent,
    alignItems: 'center',
  },
  badgeText: {
    fontSize: 9,
    fontWeight: '700',
    color: colors.onAccent,
  },
});
