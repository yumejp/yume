import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import type { DictEntry, ExampleSentence, WordStatusValue } from '../api/types';
import type { WordStatus } from '../data/types';
import { EXAMPLES_CREDIT, JMDICT_CREDIT } from '../lib/credits';
import { posLabel } from '../lib/pos';
import { FreqBadge } from './FreqBadge';
import { useServer } from '../server/ServerContext';
import { orderEntries } from '../lib/definitions';
import { useSettings, useTextScale } from '../settings/SettingsContext';
import { colors, font, fontJa, langJa, radius, spacing } from '../theme';

const STATUS_OPTIONS: { key: WordStatus; label: string; value: WordStatusValue; color: string }[] = [
  { key: 'known', label: 'Known', value: 2, color: colors.statusKnown },
  { key: 'learning', label: 'Learning', value: 1, color: colors.statusLearning },
  { key: 'unknown', label: 'Unknown', value: 0, color: colors.statusUnknown },
  { key: 'ignored', label: 'Ignore', value: 3, color: colors.statusIgnored },
];

/**
 * Tap-a-word dictionary popup for the vocab list. Looks the word up in JMdict, shows
 * readings/senses + corpus examples, and lets you re-classify its status
 * inline (persisted via set_word_status; parent refreshes the row).
 */
export function WordPopup({
  lemma,
  status,
  visible,
  onClose,
  onStatusChange,
}: {
  lemma: string | null;
  status: WordStatus;
  visible: boolean;
  onClose: () => void;
  onStatusChange?: (lemma: string, next: WordStatus) => void;
}) {
  const server = useServer();
  const live = server.live;
  const jpScale = useTextScale();
  const definitionLang = useSettings().settings.definitionLang;

  const [entries, setEntries] = useState<DictEntry[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [current, setCurrent] = useState<WordStatus>(status);
  const [examples, setExamples] = useState<ExampleSentence[] | null>(null);
  const [examplesOpen, setExamplesOpen] = useState(false);
  const [examplesLoading, setExamplesLoading] = useState(false);

  useEffect(() => setCurrent(status), [status, lemma]);

  // Reset transient view state + look the word up whenever the popup opens.
  useEffect(() => {
    if (!visible || !lemma) return;
    setExamples(null);
    setExamplesOpen(false);
    if (!live) {
      setEntries([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setEntries(null);
    server
      .client!.lookup([lemma])
      .then((res) => {
        if (!cancelled) setEntries(orderEntries(res.flatMap((r) => r.entries), definitionLang));
      })
      .catch(() => {
        if (!cancelled) setEntries([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [visible, lemma, live, server.client]);

  const setStatus = (opt: (typeof STATUS_OPTIONS)[number]) => {
    if (!lemma) return;
    setCurrent(opt.key);
    if (live) void server.client!.setWordStatus(lemma, opt.value).catch(() => {});
    onStatusChange?.(lemma, opt.key);
  };

  const toggleExamples = () => {
    if (examplesOpen) {
      setExamplesOpen(false);
      return;
    }
    setExamplesOpen(true);
    if (examples != null || !live || !lemma) return;
    setExamplesLoading(true);
    server
      .client!.examples([lemma], 3)
      .then((ex) => setExamples(ex))
      .catch(() => setExamples([]))
      .finally(() => setExamplesLoading(false));
  };

  const reading = entries?.[0]?.kanji?.length ? entries[0].readings ?? [] : [];

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <Pressable style={StyleSheet.absoluteFill} onPress={onClose} accessibilityLabel="Close" />
        <View style={styles.sheet}>
          <View style={styles.headRow}>
            <Text {...langJa} style={[styles.lemma, jpScale !== 1 && { fontSize: Math.round(font.xl * jpScale) }]}>
              {lemma}
            </Text>
            {reading.length > 0 ? (
              <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(font.md * jpScale) }]}>
                {reading.join('、')}
              </Text>
            ) : null}
            {/* Server sorts entries by commonness — first entry's priority is
                the word's rank. Only after entries load (no rare→real flip). */}
            {entries && entries.length > 0 ? (
              <View style={styles.freqSlot}>
                <FreqBadge priority={entries[0].priority} common={entries[0].common} />
              </View>
            ) : null}
          </View>

          {/* Status chips */}
          <View style={styles.statusRow}>
            {STATUS_OPTIONS.map((o) => {
              const active = current === o.key;
              return (
                <Pressable
                  key={o.key}
                  style={[styles.statusChip, active && { borderColor: o.color, backgroundColor: o.color + '22' }]}
                  onPress={() => setStatus(o)}
                  accessibilityRole="button"
                  accessibilityState={{ selected: active }}
                >
                  <View style={[styles.statusDot, { backgroundColor: o.color }]} />
                  <Text style={[styles.statusLabel, active && { color: colors.textPrimary, fontWeight: '700' }]}>
                    {o.label}
                  </Text>
                </Pressable>
              );
            })}
          </View>

          <ScrollView style={styles.body} contentContainerStyle={styles.bodyContent}>
            {loading ? (
              <ActivityIndicator color={colors.textSecondary} style={{ marginTop: spacing.lg }} />
            ) : entries && entries.length > 0 ? (
              entries.map((entry) => (
                <View key={entry.id} style={styles.entry}>
                  {(() => {
                    let prevPos = '';
                    return entry.senses.map((sense, i) => {
                      const pos = posLabel(sense.pos);
                      const showPos = pos !== '' && pos !== prevPos;
                      if (pos) prevPos = pos;
                      return (
                        <View key={i} style={styles.sense}>
                          {showPos ? <Text style={styles.pos}>{pos}</Text> : null}
                          <Text style={styles.gloss}>
                            <Text style={styles.senseNum}>{i + 1}. </Text>
                            {(sense.glosses ?? []).join('; ')}
                          </Text>
                        </View>
                      );
                    });
                  })()}
                </View>
              ))
            ) : (
              <Text style={styles.empty}>
                {!live ? 'Connect to your server to see definitions.' : 'No dictionary entry found.'}
              </Text>
            )}

            {entries && entries.length > 0 ? (
              <>
                <Pressable style={styles.examplesBtn} onPress={toggleExamples} accessibilityRole="button">
                  <Text style={styles.examplesBtnLabel}>
                    {examplesOpen ? 'Hide examples' : 'Examples'}
                  </Text>
                </Pressable>
                {examplesOpen ? (
                  examplesLoading ? (
                    <ActivityIndicator color={colors.textSecondary} />
                  ) : examples && examples.length > 0 ? (
                    <>
                      {examples.map((ex, i) => (
                        <View key={i} style={styles.example}>
                          <Text {...langJa} style={styles.exampleJa}>{ex.text}</Text>
                          <Text style={styles.exampleEn}>{ex.translation}</Text>
                        </View>
                      ))}
                      <Text style={styles.credit}>{EXAMPLES_CREDIT}</Text>
                    </>
                  ) : (
                    <Text style={styles.empty}>No example sentences.</Text>
                  )
                ) : null}
              </>
            ) : null}
          </ScrollView>

          {entries && entries.length > 0 ? <Text style={styles.footerCredit}>{JMDICT_CREDIT}</Text> : null}

          <Pressable style={styles.closeRow} onPress={onClose} accessibilityRole="button">
            <Text style={styles.closeLabel}>Close</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: 'flex-end',
    backgroundColor: colors.backdrop,
  },
  sheet: {
    backgroundColor: colors.surface1,
    borderTopColor: colors.border,
    borderTopWidth: 1,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.md,
    paddingBottom: spacing.lg,
    maxHeight: '80%',
  },
  headRow: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: spacing.sm,
    flexWrap: 'wrap',
  },
  // Pushes the freq badge to the row's right edge (lemma + reading lead).
  freqSlot: {
    marginLeft: 'auto',
    alignSelf: 'center',
  },
  lemma: {
    fontFamily: fontJa,
    fontSize: font.xl,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  reading: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.accentText,
  },
  statusRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    marginTop: spacing.md,
  },
  statusChip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: 12,
    paddingVertical: 6,
    backgroundColor: colors.surface2,
  },
  statusDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  statusLabel: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  body: {
    marginTop: spacing.md,
  },
  bodyContent: {
    paddingBottom: spacing.sm,
  },
  entry: {
    gap: spacing.xs,
    marginBottom: spacing.sm,
  },
  sense: {
    gap: 3,
    marginTop: spacing.xs,
  },
  pos: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  // Full-strength text — the definition is the sheet's primary content.
  gloss: {
    fontSize: font.md,
    color: colors.textPrimary,
  },
  senseNum: {
    color: colors.textTertiary,
  },
  empty: {
    fontSize: font.sm,
    color: colors.textTertiary,
    marginTop: spacing.sm,
  },
  examplesBtn: {
    alignSelf: 'flex-start',
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 6,
    backgroundColor: colors.surface2,
    marginTop: spacing.sm,
    marginBottom: spacing.sm,
  },
  examplesBtnLabel: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  example: {
    gap: 2,
    marginBottom: spacing.sm,
  },
  exampleJa: {
    fontFamily: fontJa,
    fontSize: font.md,
    color: colors.textPrimary,
  },
  exampleEn: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
  credit: {
    fontSize: font.xs,
    color: colors.textTertiary,
    marginBottom: spacing.sm,
  },
  // Outside the scroll view, so the dictionary credit stays on screen.
  footerCredit: {
    fontSize: font.xs,
    color: colors.textTertiary,
    textAlign: 'center',
    paddingTop: spacing.sm,
  },
  closeRow: {
    alignItems: 'center',
    paddingTop: spacing.md,
  },
  closeLabel: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
});
