import { useCallback, useEffect, useState } from 'react';
import { FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import Swipeable from 'react-native-gesture-handler/Swipeable';
import type { WatchWord } from '../api/types';
import { Icon } from '../components/Icon';
import { MeetingMode } from '../components/MeetingMode';
import { meetingCapability } from '../lib/meetingCapability';
import { invalidate, useInvalidationVersion } from '../data/queryStore';
import { addSuggestionToDeck } from '../lib/addSuggestion';
import { errorText } from '../lib/errors';
import { useServer } from '../server/ServerContext';
import { useTextScale } from '../settings/SettingsContext';
import { colors, font, fontJa, langJa, radius, spacing } from '../theme';

/**
 * Library › Suggested displays AI tutor suggestions from `watch_words`.
 * Suggestions enter review only after "Add to deck". Each action removes the
 * row from the pending list:
 *   Add    → save a card (set Learning) + drop the suggestion
 *   Known  → mark known (server auto-retires the suggestion)
 *   Ignore → mark ignored (server auto-retires)
 *   ✕      → just withdraw the suggestion (no status change)
 */
export default function SuggestedScreen() {
  const server = useServer();
  const live = server.live;
  const jpScale = useTextScale();

  const [words, setWords] = useState<WatchWord[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [meetingOpen, setMeetingOpen] = useState(false);
  const capability = meetingCapability(server.devUser != null || server.isOwner);

  const load = useCallback(() => {
    if (!live) { setWords(null); return; }
    setLoading(true);
    server.client!.listWatchWords()
      .then((w) => { setWords(w); setError(null); })
      .catch((e) => setError(errorText(e)))
      .finally(() => setLoading(false));
  }, [live, server.client]);

  // Keep current suggestions visible while the list refreshes.
  const suggestedVersion = useInvalidationVersion('suggested');
  useEffect(() => { load(); }, [load, suggestedVersion]);

  const drop = (lemma: string) =>
    setWords((prev) => (prev ? prev.filter((w) => w.lemma !== lemma) : prev));
  const mark = (lemma: string, on: boolean) =>
    setBusy((prev) => {
      const n = new Set(prev);
      if (on) n.add(lemma); else n.delete(lemma);
      return n;
    });

  const addToDeck = async (w: WatchWord) => {
    if (!server.client) return;
    mark(w.lemma, true);
    try {
      await addSuggestionToDeck(server.client, w);
      drop(w.lemma);
      invalidate('vocab', 'stats');
    } catch (e) {
      setError(errorText(e));
    } finally {
      mark(w.lemma, false);
    }
  };

  const setStatus = async (w: WatchWord, status: 2 | 3) => {
    if (!server.client) return;
    mark(w.lemma, true);
    try {
      await server.client.setWordStatus(w.lemma, status); // auto-retires suggestion
      drop(w.lemma);
      invalidate('vocab', 'stats');
    } catch (e) {
      setError(errorText(e));
    } finally {
      mark(w.lemma, false);
    }
  };

  const dismiss = async (w: WatchWord) => {
    if (!server.client) return;
    mark(w.lemma, true);
    try {
      await server.client.removeWatchWord(w.lemma);
      drop(w.lemma);
    } catch (e) {
      setError(errorText(e));
    } finally {
      mark(w.lemma, false);
    }
  };

  if (!live) {
    return (
      <View style={styles.root}>
        <Text style={styles.empty}>
          Connect to your server to see words your tutor wants you to notice.
        </Text>
        <Pressable
          style={styles.retry}
          onPress={() => { void server.refresh(); }}
          accessibilityRole="button"
        >
          <Text style={styles.retryLabel}>Retry connection</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.root}>
      {/* Text scanning produces suggestions, so its entry point lives here. */}
      {capability !== 'none' ? (
        <Pressable
          style={styles.meetingBtn}
          onPress={() => setMeetingOpen(true)}
          accessibilityRole="button"
        >
          <Icon name="microphone-outline" size={16} color={colors.accentText} />
          <Text style={styles.meetingLabel}>Scan Japanese text</Text>
        </Pressable>
      ) : null}
      <MeetingMode
        visible={meetingOpen}
        capability={capability}
        onClose={() => { setMeetingOpen(false); load(); }}
      />
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <FlatList
        data={words ?? []}
        keyExtractor={(w) => w.lemma}
        contentContainerStyle={styles.list}
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        onRefresh={load}
        refreshing={loading}
        ListEmptyComponent={
          loading ? (
            <Text style={styles.empty}>Loading…</Text>
          ) : (
            <View style={styles.emptyBlock}>
              <Text style={styles.empty}>No suggested words yet.</Text>
              <Text style={styles.emptyHint}>
                Ask your AI tutor for suggestions, scan Japanese text, or add
                words directly from Dictionary.
              </Text>
            </View>
          )
        }
        renderItem={({ item }) => {
          const working = busy.has(item.lemma);
          return (
            // Swipe right = add to deck, swipe left = dismiss: the one-thumb
            // path. The buttons stay for discoverability and for Known /
            // Ignore, which are deliberate enough to deserve a tap.
            <Swipeable
              enabled={!working}
              friction={2}
              leftThreshold={72}
              rightThreshold={72}
              overshootLeft={false}
              overshootRight={false}
              renderLeftActions={() => (
                <View style={[styles.swipe, styles.swipeAdd]}>
                  <Icon name="plus" size={16} color={colors.accentText} />
                  <Text style={[styles.swipeLabel, { color: colors.accentText }]}>Add to deck</Text>
                </View>
              )}
              renderRightActions={() => (
                <View style={[styles.swipe, styles.swipeDismiss]}>
                  <Text style={styles.swipeLabel}>Dismiss</Text>
                  <Icon name="close" size={16} color={colors.textSecondary} />
                </View>
              )}
              onSwipeableOpen={(direction, ref) => {
                ref.close();
                if (direction === 'left') void addToDeck(item);
                else void dismiss(item);
              }}
            >
            <View style={[styles.row, working && styles.rowBusy]}>
              {/* Furigana line — skipped for kana-only words where the
                  reading just repeats the lemma. */}
              {item.readings?.[0] && item.readings[0] !== item.lemma ? (
                <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(font.sm * jpScale) }]}>
                  {item.readings[0]}
                </Text>
              ) : null}
              <Text {...langJa} style={[styles.lemma, jpScale !== 1 && { fontSize: Math.round(font.xl * jpScale) }]}>
                {item.lemma}
              </Text>
              {item.note ? <Text style={styles.note}>{item.note}</Text> : null}
              <View style={styles.actions}>
                <Pressable
                  style={[styles.btn, styles.btnAdd]}
                  disabled={working}
                  onPress={() => addToDeck(item)}
                  accessibilityRole="button"
                >
                  <View style={styles.btnAddInner}>
                    <Icon name="plus" size={14} color={colors.accentText} />
                    <Text style={[styles.btnLabel, styles.btnAddLabel]}>Add to deck</Text>
                  </View>
                </Pressable>
                <Pressable style={styles.btn} disabled={working} onPress={() => setStatus(item, 2)}>
                  <Text style={[styles.btnLabel, { color: colors.statusKnown }]}>Known</Text>
                </Pressable>
                <Pressable style={styles.btn} disabled={working} onPress={() => setStatus(item, 3)}>
                  <Text style={[styles.btnLabel, { color: colors.statusIgnored }]}>Ignore</Text>
                </Pressable>
              </View>
            </View>
            </Swipeable>
          );
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  meetingBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    marginHorizontal: spacing.lg,
    marginBottom: spacing.sm,
    paddingVertical: 10,
    paddingHorizontal: spacing.md,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface1,
  },
  meetingLabel: {
    fontSize: font.sm,
    color: colors.accentText,
    fontWeight: '600',
  },
  root: { flex: 1 },
  list: { paddingHorizontal: spacing.lg, paddingBottom: spacing.lg, flexGrow: 1 },
  separator: { height: spacing.sm },
  row: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: 6,
  },
  rowBusy: { opacity: 0.5 },
  swipe: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: spacing.md,
    borderRadius: radius.md,
  },
  swipeAdd: { backgroundColor: colors.accentMuted, marginRight: spacing.sm },
  swipeDismiss: { backgroundColor: colors.surface2, marginLeft: spacing.sm },
  swipeLabel: { fontSize: font.sm, fontWeight: '600', color: colors.textSecondary },
  reading: {
    fontFamily: fontJa,
    fontSize: font.sm,
    color: colors.accentText,
    marginBottom: -4, // sit tight over the lemma like ruby text
  },
  lemma: {
    fontFamily: fontJa,
    fontSize: font.xl,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  note: { fontSize: font.sm, color: colors.textSecondary },
  actions: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    marginTop: spacing.xs,
    flexWrap: 'wrap',
  },
  btn: {
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.sm,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
  },
  btnAdd: { borderColor: colors.accent, backgroundColor: colors.accentMuted },
  btnAddInner: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  btnLabel: { fontSize: font.sm, fontWeight: '600' },
  btnAddLabel: { color: colors.accentText },
  error: {
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.xs,
    fontSize: font.xs,
    color: colors.statusUnknown,
  },
  empty: {
    marginTop: spacing.xl,
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.sm,
  },
  emptyBlock: {
    paddingHorizontal: spacing.lg,
    gap: spacing.sm,
    alignItems: 'center',
  },
  emptyHint: {
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.sm,
    lineHeight: font.sm + 7,
  },
  retry: {
    alignSelf: 'center',
    marginTop: spacing.md,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.lg,
    paddingVertical: 8,
  },
  retryLabel: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
});
