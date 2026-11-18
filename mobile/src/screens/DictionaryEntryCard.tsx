import { useState } from 'react';
import { ActivityIndicator, Pressable, Text, View } from 'react-native';
import type { DictEntry, ExampleSentence } from '../api/types';
import { FreqBadge } from '../components/FreqBadge';
import { EXAMPLES_CREDIT } from '../lib/credits';
import { posLabel } from '../lib/pos';
import { useTextScale } from '../settings/SettingsContext';
import { colors, font, langJa } from '../theme';
import { styles } from './DictionaryScreen.styles';

const SENSE_PREVIEW = 3;

export function dictionaryHeadword(entry: DictEntry): string {
  return entry.kanji?.[0] ?? entry.readings?.[0] ?? '';
}

export function DictionaryEntryCard({
  entry,
  expanded,
  examples,
  examplesLoading,
  examplesFailed,
  saved,
  saving,
  onToggleExamples,
  onSave,
  onRemove,
}: {
  entry: DictEntry;
  expanded: boolean;
  examples: ExampleSentence[] | undefined;
  examplesLoading: boolean;
  examplesFailed: boolean;
  saved: boolean;
  saving: boolean;
  onToggleExamples: () => void;
  onSave: () => void;
  onRemove: () => void;
}) {
  const head = dictionaryHeadword(entry);
  const readings = entry.kanji?.length ? entry.readings ?? [] : [];
  const jpScale = useTextScale();
  const [allSenses, setAllSenses] = useState(false);
  const senses = allSenses ? entry.senses : entry.senses.slice(0, SENSE_PREVIEW);
  const hiddenSenses = entry.senses.length - SENSE_PREVIEW;

  let previousPos = '';

  return (
    <View style={styles.card}>
      <View style={styles.cardHead}>
        <View style={styles.headLeft}>
          <Text {...langJa} style={[styles.head, jpScale !== 1 && { fontSize: Math.round(font.xl * jpScale) }]}>
            {head}
          </Text>
          {readings.length > 0 ? (
            <Text {...langJa} style={[styles.readings, jpScale !== 1 && { fontSize: Math.round(font.md * jpScale) }]}>
              {readings.join('、')}
            </Text>
          ) : null}
        </View>
        <FreqBadge priority={entry.priority} common={entry.common} />
      </View>

      {senses.map((sense, index) => {
        const pos = posLabel(sense.pos);
        const showPos = pos !== '' && pos !== previousPos;
        if (pos) previousPos = pos;
        return (
          <View key={index} style={styles.sense}>
            {showPos ? <Text style={styles.pos}>{pos}</Text> : null}
            <Text style={styles.gloss}>
              <Text style={styles.senseNum}>{index + 1}. </Text>
              {(sense.glosses ?? []).join('; ')}
            </Text>
          </View>
        );
      })}
      {hiddenSenses > 0 ? (
        <Pressable
          onPress={() => setAllSenses((show) => !show)}
          accessibilityRole="button"
          accessibilityState={{ expanded: allSenses }}
        >
          <Text style={styles.moreSenses}>
            {allSenses ? 'Show fewer senses' : `+${hiddenSenses} more ${hiddenSenses === 1 ? 'sense' : 'senses'}`}
          </Text>
        </Pressable>
      ) : null}

      <View style={styles.actions}>
        <Pressable style={styles.actionBtn} onPress={onToggleExamples} accessibilityRole="button">
          <Text style={styles.actionText}>{expanded ? 'Hide examples' : 'Examples'}</Text>
        </Pressable>
        {saved ? (
          <>
            <View style={[styles.actionBtn, styles.actionBtnDone]}>
              <Text style={[styles.actionText, styles.actionTextDone]}>In library</Text>
            </View>
            <Pressable style={styles.actionBtn} onPress={onRemove} disabled={saving}
              accessibilityRole="button" accessibilityLabel={`Remove ${head} from deck`}>
              <Text style={styles.actionText}>{saving ? 'Removing…' : 'Remove'}</Text>
            </Pressable>
          </>
        ) : (
          <Pressable style={styles.actionBtn} onPress={onSave} disabled={saving}
            accessibilityRole="button" accessibilityLabel="Add to deck">
            <Text style={styles.actionText}>{saving ? 'Saving…' : 'Add to deck'}</Text>
          </Pressable>
        )}
      </View>

      {expanded ? (
        <View style={styles.examples}>
          {examplesLoading ? (
            <ActivityIndicator color={colors.textSecondary} />
          ) : examples && examples.length > 0 ? (
            <>
              {examples.map((example, index) => (
                <View key={index} style={styles.example}>
                  <Text {...langJa} style={styles.exampleJa}>{example.text}</Text>
                  <Text style={styles.exampleEn}>{example.translation}</Text>
                </View>
              ))}
              <Text style={styles.exampleCredit}>{EXAMPLES_CREDIT}</Text>
            </>
          ) : examplesFailed ? (
            <Text style={styles.caption}>Couldn’t load examples. Tap Examples to try again.</Text>
          ) : (
            <Text style={styles.caption}>No example sentences.</Text>
          )}
        </View>
      ) : null}
    </View>
  );
}
