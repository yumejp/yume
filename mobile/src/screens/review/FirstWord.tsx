import { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import type { WatchWord } from '../../api/types';
import { MoonMark } from '../../components/MoonMark';
import { invalidate, useInvalidationVersion } from '../../data/queryStore';
import { addSuggestionToDeck } from '../../lib/addSuggestion';
import { errorText } from '../../lib/errors';
import { useServer } from '../../server/ServerContext';
import { colors, font, radius, spacing } from '../../theme';
import { shared } from './shared';

const MAX_SHOWN = 8;

/** The empty library. With tutor suggestions, tapping one saves it as the
 *  first card; without, the button opens the dictionary. */
export function FirstWord({ onFindWord }: { onFindWord: () => void }) {
  const server = useServer();
  const [words, setWords] = useState<WatchWord[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const version = useInvalidationVersion('suggested');
  useEffect(() => {
    if (!server.live || !server.client) { setWords([]); return; }
    let cancelled = false;
    server.client.listWatchWords()
      .then((w) => { if (!cancelled) setWords(w.filter((x) => x.status === 0).slice(0, MAX_SHOWN)); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [server.live, server.client, version]);

  const add = async (w: WatchWord) => {
    if (!server.client || busy) return;
    setBusy(w.lemma);
    setError(null);
    try {
      await addSuggestionToDeck(server.client, w);
      invalidate('vocab', 'stats', 'suggested');
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <View style={shared.center}>
      <View style={styles.top} />
      <MoonMark />
      <Text style={shared.dueHeadline}>Your library is empty</Text>
      {words.length > 0 ? (
        <>
          <Text style={shared.dueSub}>Suggested words</Text>
          <View style={styles.chips}>
            {words.map((w) => (
              <Pressable
                key={w.lemma}
                onPress={() => void add(w)}
                disabled={busy != null}
                style={({ pressed }) => [styles.chip, (pressed || busy === w.lemma) && styles.chipPressed]}
                accessibilityRole="button"
                accessibilityLabel={`Add ${w.lemma} to your library`}
              >
                <Text style={styles.chipText}>{w.lemma}</Text>
                {w.readings?.[0] && w.readings[0] !== w.lemma ? <Text style={styles.chipReading}>{w.readings[0]}</Text> : null}
              </Pressable>
            ))}
          </View>
          <Pressable onPress={onFindWord} accessibilityRole="button" hitSlop={8}>
            <Text style={styles.link}>Find another word</Text>
          </Pressable>
        </>
      ) : (
        <Pressable style={shared.cta} onPress={onFindWord} accessibilityRole="button">
          <Text style={shared.ctaLabel}>Find a word</Text>
        </Pressable>
      )}
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <View style={styles.bottom} />
    </View>
  );
}

const styles = StyleSheet.create({
  top: { flex: 2 },
  bottom: { flex: 3 },
  chips: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: spacing.sm,
    maxWidth: 340,
    marginTop: spacing.sm,
    marginBottom: spacing.md,
  },
  chip: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: 6,
    paddingVertical: 8,
    paddingHorizontal: 14,
    borderRadius: radius.pill,
    backgroundColor: colors.surface2,
  },
  chipPressed: { backgroundColor: colors.accentMuted },
  chipText: { fontSize: font.lg, color: colors.textPrimary },
  chipReading: { fontSize: font.xs, color: colors.textSecondary },
  link: { fontSize: font.sm, color: colors.accentText, padding: spacing.xs },
  error: { marginTop: spacing.sm, fontSize: font.sm, color: colors.statusUnknown, textAlign: 'center' },
});
