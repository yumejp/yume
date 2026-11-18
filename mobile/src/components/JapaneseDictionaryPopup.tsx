import { useEffect, useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, Text, View } from 'react-native';
import type { YumeClient } from '../api/client';
import type { DictEntry, Token } from '../api/types';
import type { WordStatus } from '../data/types';
import { kataToHira } from '../lib/kana';
import { colors, langJa, statusColor } from '../theme';
import { FreqBadge } from './FreqBadge';
import { styles } from './JapaneseSentence.styles';

const STATUS_CHIPS: { status: WordStatus; label: string }[] = [
  { status: 'known', label: 'Known' },
  { status: 'learning', label: 'Learning' },
  { status: 'unknown', label: 'Unknown' },
  { status: 'ignored', label: 'Ignore' },
];

export function JapaneseDictionaryPopup({
  token,
  client,
  status,
  onSetStatus,
  onClose,
}: {
  token: Token | null;
  client: YumeClient;
  status: WordStatus;
  onSetStatus: (status: WordStatus) => void;
  onClose: () => void;
}) {
  const [entries, setEntries] = useState<DictEntry[] | null>(null);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    setEntries(null);
    const queries = [...new Set([token.lemma || token.surface, token.surface].filter(Boolean))];
    client.lookup(queries)
      .then((results) => {
        if (cancelled) return;
        const seen = new Set<number>();
        const merged: DictEntry[] = [];
        for (const result of results) {
          for (const entry of result.entries) {
            if (!seen.has(entry.id)) {
              seen.add(entry.id);
              merged.push(entry);
            }
          }
        }
        setEntries(merged);
      })
      .catch(() => { if (!cancelled) setEntries([]); });
    return () => { cancelled = true; };
  }, [token, client]);

  return (
    <Modal visible={token != null} transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={styles.sheet} onPress={(event) => event.stopPropagation()}>
          {token ? (
            <ScrollView>
              <View style={styles.headRow}>
                <Text {...langJa} style={styles.headword}>{token.surface}</Text>
                {entries && entries.length > 0 ? <FreqBadge priority={entries[0].priority} common={entries[0].common} /> : null}
              </View>
              {token.reading ? <Text {...langJa} style={styles.headReading}>{kataToHira(token.reading)}</Text> : null}

              <View style={styles.chips}>
                {STATUS_CHIPS.map((chip) => {
                  const active = chip.status === status;
                  const accent = chip.status === 'unknown'
                    ? colors.textTertiary
                    : statusColor(chip.status);
                  return (
                    <Pressable
                      key={chip.status}
                      onPress={() => onSetStatus(chip.status)}
                      accessibilityRole="button"
                      style={[
                        styles.chip,
                        { borderColor: accent },
                        active && { backgroundColor: accent },
                      ]}
                    >
                      <Text style={[styles.chipLabel, { color: active ? colors.surface0 : accent }]}>
                        {chip.label}
                      </Text>
                    </Pressable>
                  );
                })}
              </View>

              <View style={styles.body}>
                {entries === null ? (
                  <View style={styles.loadingWrap}>
                    <ActivityIndicator color={colors.textTertiary} />
                  </View>
                ) : entries.length === 0 ? (
                  <Text style={styles.dim}>No dictionary entry found.</Text>
                ) : (
                  entries.map((entry) => (
                    <View key={entry.id} style={styles.entry}>
                      {(entry.kanji?.length || entry.readings?.length) ? (
                        <Text {...langJa} style={styles.entryHead}>
                          {[entry.kanji?.[0], entry.readings?.[0]].filter(Boolean).join('  ')}
                        </Text>
                      ) : null}
                      {entry.senses.map((sense, index) => (
                        <View key={index} style={styles.sense}>
                          {sense.pos?.length ? <Text style={styles.pos}>{sense.pos.join(', ')}</Text> : null}
                          <Text style={styles.gloss}>
                            {index + 1}. {(sense.glosses ?? []).join('; ')}
                          </Text>
                        </View>
                      ))}
                    </View>
                  ))
                )}
              </View>
            </ScrollView>
          ) : null}

          <Pressable style={styles.close} onPress={onClose} accessibilityRole="button">
            <Text style={styles.closeLabel}>Close</Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
