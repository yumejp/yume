import { useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import type { MinedWord } from '../api/types';
import { invalidate } from '../data/queryStore';
import type { MeetingCapability } from '../lib/meetingCapability';
import { useServer } from '../server/ServerContext';
import { colors, font, radius, spacing } from '../theme';
import { errorText } from '../lib/errors';
import { Icon } from './Icon';

/**
 * Finds recurring unfamiliar words in submitted Japanese text. The server
 * processes the text without storing it; only accepted lemmas are saved.
 */
export function MeetingMode({
  visible,
  capability,
  onClose,
}: {
  visible: boolean;
  capability: MeetingCapability;
  onClose: () => void;
}) {
  const server = useServer();
  const [transcript, setTranscript] = useState('');
  const [mining, setMining] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<MinedWord[] | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [added, setAdded] = useState<number | null>(null);
  // True when results come from the relaxed single-occurrence pass.
  const [heardOnce, setHeardOnce] = useState(false);

  const reset = () => {
    setTranscript('');
    setResults(null);
    setHeardOnce(false);
    setPicked(new Set());
    setError(null);
    setAdded(null);
  };

  const mine = async () => {
    if (!server.client || !transcript.trim() || mining) return;
    setMining(true);
    setError(null);
    try {
      const resp = await server.client.mineTranscript(transcript);
      let words = resp.words ?? [];
      let once = false;
      // Relax longer inputs to single occurrences when nothing repeats.
      if (words.length === 0 && transcript.trim().length >= 40) {
        const relaxed = await server.client.mineTranscript(transcript, 'ja', { minCount: 1 });
        words = relaxed.words ?? [];
        once = words.length > 0;
      }
      setHeardOnce(once);
      setResults(words);
      setPicked(new Set(words.map((w) => w.lemma)));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setMining(false);
    }
  };

  const toggle = (lemma: string) =>
    setPicked((prev) => {
      const n = new Set(prev);
      if (n.has(lemma)) n.delete(lemma);
      else n.add(lemma);
      return n;
    });

  const accept = async () => {
    if (!server.client || picked.size === 0) return;
    setMining(true);
    try {
      const chosen = (results ?? []).filter((w) => picked.has(w.lemma));
      const n = await server.client.addWatchWords(
        chosen.map((w) => ({
          lemma: w.lemma,
          note: `appeared ${w.count}× in scanned text${w.sentence ? `: 「${w.sentence.slice(0, 40)}」` : ''}`,
        })),
      );
      setAdded(n);
      invalidate('suggested');
    } catch (e) {
      setError(errorText(e));
    } finally {
      setMining(false);
    }
  };

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <View style={styles.sheet}>
          <View style={styles.head}>
            <Text style={styles.title}>Scan Japanese text</Text>
            <Pressable onPress={() => { reset(); onClose(); }} accessibilityRole="button" hitSlop={8}>
              <Text style={styles.close}>Done</Text>
            </Pressable>
          </View>

          {added != null ? (
            <View style={styles.doneWrap}>
              <Text style={styles.doneText}>
                {added} {added === 1 ? 'word' : 'words'} added to Suggested.
              </Text>
              <Pressable style={styles.cta} onPress={() => { reset(); onClose(); }} accessibilityRole="button">
                <Text style={styles.ctaLabel}>Done</Text>
              </Pressable>
            </View>
          ) : results ? (
            <>
              <Text style={styles.hint}>
                {results.length === 0
                  ? 'No unfamiliar word appeared more than once.'
                  : heardOnce
                    ? `Nothing came up twice. ${results.length} unfamiliar ${results.length === 1 ? 'word' : 'words'} heard once. Uncheck any you don’t want.`
                    : `${results.length} repeated ${results.length === 1 ? 'word' : 'words'} found. Uncheck any you don’t want.`}
              </Text>
              <ScrollView style={styles.list}>
                {results.map((w) => {
                  const on = picked.has(w.lemma);
                  return (
                    <Pressable
                      key={w.lemma}
                      style={[styles.row, !on && styles.rowOff]}
                      onPress={() => toggle(w.lemma)}
                      accessibilityRole="checkbox"
                      accessibilityState={{ checked: on }}
                    >
                      <View style={[styles.check, on && styles.checkOn]}>
                        {on ? <Icon name="check" size={14} color={colors.onAccent} /> : null}
                      </View>
                      <View style={styles.rowMain}>
                        <View style={styles.rowHead}>
                          <Text style={styles.lemma}>{w.lemma}</Text>
                          {w.readings?.[0] ? <Text style={styles.reading}>{w.readings[0]}</Text> : null}
                          {w.status === 1 ? <Text style={styles.tagLearning}>learning</Text> : null}
                          <Text style={styles.count}>{w.count}×</Text>
                        </View>
                        {w.glosses?.length ? (
                          <Text style={styles.gloss} numberOfLines={1}>{w.glosses.join(' · ')}</Text>
                        ) : null}
                        {w.sentence ? (
                          <Text style={styles.sentence} numberOfLines={1}>「{w.sentence}」</Text>
                        ) : null}
                      </View>
                    </Pressable>
                  );
                })}
              </ScrollView>
              {results.length > 0 ? (
                <Pressable
                  style={[styles.cta, (picked.size === 0 || mining) && styles.ctaDisabled]}
                  onPress={accept}
                  disabled={picked.size === 0 || mining}
                  accessibilityRole="button"
                >
                  <Text style={styles.ctaLabel}>
                    {mining ? 'Adding…' : `Add ${picked.size} ${picked.size === 1 ? 'word' : 'words'}`}
                  </Text>
                </Pressable>
              ) : null}
            </>
          ) : (
            <>
              {capability === 'native' ? (
                <Text style={styles.hint}>
                  Recording is not available yet. Paste Japanese text instead.
                </Text>
              ) : (
                <Text style={styles.hint}>
                  Paste Japanese text from a transcript, article, or conversation.
                  Yume finds unfamiliar words that repeat. Your pasted text is not saved.
                </Text>
              )}
              <TextInput
                style={styles.input}
                multiline
                placeholder="Paste Japanese text here…"
                placeholderTextColor={colors.textTertiary}
                value={transcript}
                onChangeText={setTranscript}
              />
              {error ? <Text style={styles.error}>{error}</Text> : null}
              <Pressable
                style={[styles.cta, (!transcript.trim() || mining) && styles.ctaDisabled]}
                onPress={mine}
                disabled={!transcript.trim() || mining}
                accessibilityRole="button"
              >
                {mining ? (
                  <ActivityIndicator color={colors.textPrimary} />
                ) : (
                  <Text style={styles.ctaLabel}>Find repeated words</Text>
                )}
              </Pressable>
            </>
          )}
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: colors.backdrop,
    justifyContent: 'flex-end',
  },
  sheet: {
    backgroundColor: colors.surface2,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    borderColor: colors.border,
    borderWidth: 1,
    padding: spacing.lg,
    maxHeight: '85%',
    gap: spacing.md,
  },
  head: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  title: {
    fontSize: font.lg,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  close: {
    fontSize: font.md,
    color: colors.accentText,
    fontWeight: '600',
  },
  hint: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  input: {
    minHeight: 140,
    maxHeight: 260,
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    padding: spacing.md,
    color: colors.textPrimary,
    fontSize: font.md,
    textAlignVertical: 'top',
  },
  list: {
    flexGrow: 0,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingVertical: spacing.sm,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  tagLearning: {
    fontSize: font.xs,
    color: colors.statusLearning,
    borderColor: colors.statusLearning,
    borderWidth: 1,
    borderRadius: radius.sm,
    paddingHorizontal: 6,
    paddingVertical: 1,
    marginLeft: 2,
  },
  rowOff: {
    opacity: 0.45,
  },
  check: {
    width: 22,
    height: 22,
    borderRadius: radius.sm - 2,
    borderWidth: 1.5,
    borderColor: colors.textTertiary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  checkOn: {
    backgroundColor: colors.accent,
    borderColor: colors.accentText,
  },
  rowMain: { flex: 1, gap: 2 },
  rowHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: spacing.sm,
  },
  lemma: {
    fontSize: font.lg,
    fontWeight: '600',
    color: colors.textPrimary,
  },
  reading: {
    fontSize: font.sm,
    color: colors.accentText,
  },
  count: {
    marginLeft: 'auto',
    fontSize: font.sm,
    fontWeight: '700',
    color: colors.statusLearning,
  },
  gloss: {
    fontSize: font.sm,
    color: colors.textSecondary,
  },
  sentence: {
    fontSize: font.xs,
    color: colors.textTertiary,
  },
  error: {
    fontSize: font.sm,
    color: colors.statusUnknown,
  },
  cta: {
    backgroundColor: colors.accent,
    borderRadius: radius.md,
    paddingVertical: 13,
    alignItems: 'center',
  },
  ctaDisabled: {
    opacity: 0.4,
  },
  ctaLabel: {
    color: colors.onAccent,
    fontSize: font.md,
    fontWeight: '600',
  },
  doneWrap: {
    gap: spacing.md,
    paddingVertical: spacing.md,
  },
  doneText: {
    fontSize: font.md,
    color: colors.textPrimary,
    textAlign: 'center',
  },
});
