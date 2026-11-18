import { useEffect, useState } from 'react';
import { ActivityIndicator, Linking, Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import type { YumeClient } from '../api/client';
import type { KanjiInfo } from '../api/types';
import { errorText } from '../lib/errors';
import { colors, font, fontDisplay, fontJa, langJa, radius, spacing, statusColor } from '../theme';

const STATUS = ['unknown', 'learning', 'known', 'ignored'] as const;

/** Tap a kanji tile in a mnemonic: the kanji, its meaning from the breakdown,
 *  the tutor's story for it (when one exists) and every deck word using it. */
export function KanjiSheet({
  kanji,
  meaning,
  client,
  onClose,
}: {
  kanji: string | null;
  meaning?: string;
  client: YumeClient | null;
  onClose: () => void;
}) {
  const [info, setInfo] = useState<KanjiInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    setInfo(null);
    setError(null);
    if (!kanji || !client) return;
    let cancelled = false;
    client.kanjiInfo(kanji)
      .then((i) => {
        if (cancelled) return;
        // Duplicate cards for one lemma are one word here.
        const seen = new Set<string>();
        setInfo({ ...i, words: i.words.filter((w) => !seen.has(w.lemma) && seen.add(w.lemma)) });
      })
      .catch((e) => { if (!cancelled) setError(errorText(e)); });
    return () => { cancelled = true; };
  }, [kanji, client]);

  return (
    <Modal visible={kanji != null} transparent animationType="slide" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <Pressable style={StyleSheet.absoluteFill} onPress={onClose} accessibilityLabel="Close" />
        <View style={styles.sheet}>
          <View style={styles.head}>
            <Text {...langJa} style={styles.kanji}>{kanji}</Text>
            <View style={styles.headText}>
              <Text style={styles.meaning}>{info?.dict?.meanings?.join(', ') || meaning || ''}</Text>
              {info?.dict ? (
                <Text {...langJa} style={styles.readings}>
                  {[info.dict.on?.length ? `音 ${info.dict.on.join('、')}` : '', info.dict.kun?.length ? `訓 ${info.dict.kun.join('、')}` : '']
                    .filter(Boolean)
                    .join('   ')}
                </Text>
              ) : null}
              {info?.dict ? (
                <Text style={styles.meta}>
                  {[info.dict.jlpt ? `JLPT N${info.dict.jlpt}` : '', info.dict.grade ? `grade ${info.dict.grade}` : '', info.dict.strokes ? `${info.dict.strokes} strokes` : '']
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              ) : null}
            </View>
          </View>
          <ScrollView style={styles.body} contentContainerStyle={styles.bodyInner}>
            {info ? (
              info.note ? (
                <Text style={styles.story}>{info.note}</Text>
              ) : (
                <Text style={styles.muted}>No story for this kanji yet.</Text>
              )
            ) : null}
            {info?.components?.length ? (
              <View style={styles.parts}>
                <Text style={styles.label}>Made of</Text>
                {info.components.map((c) => (
                  <View key={c.kanji} style={styles.part}>
                    <View style={styles.partHead}>
                      <Text {...langJa} style={styles.partKanji}>{c.kanji}</Text>
                      <Text style={styles.partMeaning}>{c.meaning}</Text>
                      {c.known === false ? <Text style={styles.partNew}>new to you</Text> : null}
                    </View>
                    {c.note ? <Text style={styles.partNote}>{c.note}</Text> : null}
                  </View>
                ))}
              </View>
            ) : null}
            {info && info.words.length > 0 ? (
              <View style={styles.words}>
                <Text style={styles.label}>In your deck</Text>
                {info.words.map((w) => (
                  <View key={w.card_id} style={styles.wordRow}>
                    <View style={[styles.dot, { backgroundColor: statusColor(STATUS[w.status] ?? 'unknown') }]} />
                    <Text {...langJa} style={styles.word}>{w.lemma}</Text>
                    {w.readings?.[0] ? <Text {...langJa} style={styles.reading}>{w.readings[0]}</Text> : null}
                  </View>
                ))}
              </View>
            ) : null}
            {!info && !error ? <ActivityIndicator size="small" color={colors.textTertiary} /> : null}
            {error ? <Text style={styles.error}>{error}</Text> : null}
            {info?.dict ? <Text style={styles.credit}>KANJIDIC2 / EDRDG</Text> : null}
          </ScrollView>
          <View style={styles.footer}>
            <Pressable
              onPress={() => { if (kanji) void Linking.openURL(`https://jisho.org/search/${encodeURIComponent(kanji)}%20%23kanji`); }}
              accessibilityRole="link"
              hitSlop={8}
            >
              <Text style={styles.close}>Open in Jisho</Text>
            </Pressable>
            <Pressable onPress={onClose} accessibilityRole="button" hitSlop={8}>
              <Text style={styles.close}>Close</Text>
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: colors.backdrop, justifyContent: 'flex-end' },
  sheet: {
    backgroundColor: colors.surface1,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.lg,
    paddingBottom: spacing.xl,
    maxHeight: '75%',
    gap: spacing.md,
  },
  head: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  headText: { flex: 1, gap: 2 },
  kanji: { fontFamily: fontDisplay, fontSize: 56, lineHeight: 64, color: colors.textPrimary },
  meaning: { fontSize: font.md, color: colors.textPrimary },
  readings: { fontFamily: fontJa, fontSize: font.sm, color: colors.textSecondary },
  meta: { fontSize: font.xs, color: colors.textTertiary },
  credit: { fontSize: font.xs, color: colors.textTertiary },
  footer: { flexDirection: 'row', justifyContent: 'space-between', paddingTop: spacing.xs },
  body: { flexGrow: 0 },
  bodyInner: { gap: spacing.md },
  story: { fontSize: font.md, lineHeight: font.md + 8, color: colors.textPrimary },
  label: { fontSize: font.sm, color: colors.textTertiary },
  words: { gap: spacing.sm },
  wordRow: { flexDirection: 'row', alignItems: 'baseline', gap: spacing.sm },
  dot: { width: 8, height: 8, borderRadius: 4, alignSelf: 'center' },
  word: { fontFamily: fontJa, fontSize: font.jp, color: colors.textPrimary },
  reading: { fontFamily: fontJa, fontSize: font.sm, color: colors.textSecondary },
  muted: { fontSize: font.sm, color: colors.textTertiary },
  parts: { gap: spacing.sm },
  part: { gap: 2 },
  partHead: { flexDirection: 'row', alignItems: 'baseline', gap: spacing.sm },
  partKanji: { fontFamily: fontDisplay, fontSize: 24, lineHeight: 30, color: colors.textPrimary },
  partMeaning: { fontSize: font.md, color: colors.textPrimary },
  partNew: { fontSize: font.xs, color: colors.accentText },
  partNote: { fontSize: font.sm, lineHeight: font.sm + 7, color: colors.textSecondary },
  error: { fontSize: font.sm, color: colors.statusUnknown },
  close: { fontSize: font.sm, fontWeight: '600', color: colors.accentText },
});

