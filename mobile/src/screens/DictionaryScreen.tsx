import { useEffect, useRef, useState } from 'react';
import {
  FlatList,
  Platform,
  Pressable,
  Text,
  TextInput,
  View,
} from 'react-native';
import type { DictEntry, ExampleSentence, WatchWord } from '../api/types';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DictionaryMissingNotice, missingDictionary } from '../components/DictionaryMissingNotice';
import { ScreenHeader } from '../components/ScreenHeader';
import { invalidate, useInvalidationVersion } from '../data/queryStore';
import { JMDICT_CREDIT } from '../lib/credits';
import { errorText } from '../lib/errors';
import { buildQueries, englishQuery } from '../lib/romajiSearch';
import { useServer } from '../server/ServerContext';
import { orderEntries } from '../lib/definitions';
import { useSettings } from '../settings/SettingsContext';
import { colors } from '../theme';
import { DictionaryEntryCard, dictionaryHeadword as headword } from './DictionaryEntryCard';
import { styles } from './DictionaryScreen.styles';



export default function DictionaryScreen({ onReview }: { onReview?: () => void }) {
  const server = useServer();
  const { settings } = useSettings();
  const live = server.live;
  // The notice explains a missing dictionary; per-search errors would repeat it.
  const dictMissing = missingDictionary(server.status, server.health) != null;

  const [query, setQuery] = useState('');
  // Search direction: 'jp' = look up a Japanese word (kana/kanji/romaji);
  // 'en' = reverse-search by English meaning.
  const [mode, setMode] = useState<'jp' | 'en'>('jp');
  const [entries, setEntries] = useState<DictEntry[] | null>(null);
  // Index of the first English-meaning match merged into a 日本語 → EN search.
  const [englishFrom, setEnglishFrom] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Kana we converted romaji to, shown as a hint under the search box.
  const [searchedKana, setSearchedKana] = useState<string | null>(null);

  // Per-entry expanded example sentences, lazily fetched on first expand.
  const [expanded, setExpanded] = useState<number | null>(null);
  const [examples, setExamples] = useState<Record<number, ExampleSentence[]>>({});
  const [examplesLoading, setExamplesLoading] = useState<number | null>(null);
  // Entries whose examples fetch FAILED (timeout, connection flap) — kept out
  // of `examples` so re-expanding retries, instead of a cached-forever
  // "No example sentences" that reads as missing data.
  const [examplesError, setExamplesError] = useState<Record<number, boolean>>({});

  // Entry id to saved card id. Server results include imported cards and
  // cards without a dictionary id, preventing duplicate saves.
  const [saved, setSaved] = useState<Record<number, number>>({});
  const [saving, setSaving] = useState<number | null>(null);
  const [removeTarget, setRemoveTarget] = useState<DictEntry | null>(null);

  // The search intro offers the tutor's open suggestions to look up; the
  // sample words stand in when there are none.
  const [suggested, setSuggested] = useState<WatchWord[]>([]);
  const suggestedVersion = useInvalidationVersion('suggested');
  useEffect(() => {
    if (!live || !server.client) { setSuggested([]); return; }
    let cancelled = false;
    server.client.listWatchWords()
      .then((w) => { if (!cancelled) setSuggested(w.filter((x) => x.status === 0).slice(0, 8)); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [live, server.client, suggestedVersion]);
  const [lastSaved, setLastSaved] = useState<{ id: number; word: string } | null>(null);
  const clientRef = useRef(server.client);
  clientRef.current = server.client;

  // Deck membership is per-account; entry ids are global JMdict ids, so a
  // stale "in deck" must not leak across an account switch.
  useEffect(() => {
    setSaved({});
    setRemoveTarget(null);
    setLastSaved(null);
  }, [server.client]);

  // Ask the server which results are already saved — by entry id AND by
  // lemma (the id check alone can't see imported cards).
  useEffect(() => {
    if (!live || !entries || entries.length === 0) return;
    let cancelled = false;
    server
      .client!.cardStatus(entries.map((e) => e.id), entries.map((e) => headword(e)))
      .then((resp) => {
        if (cancelled) return;
        const idHits = new Map(
          (resp.statuses ?? [])
            .filter((r) => r.card_id != null)
            .map((r) => [r.jmdict_id, r.card_id!] as const),
        );
        const lemmaHits = new Map(
          (resp.lemma_statuses ?? [])
            .filter((r) => r.card_id != null)
            .map((r) => [r.lemma, r.card_id!] as const),
        );
        setSaved((prev) => {
          const next = { ...prev };
          for (const e of entries) {
            const cardId = idHits.get(e.id) ?? lemmaHits.get(headword(e));
            if (cardId != null) next[e.id] = cardId;
          }
          return next;
        });
      })
      .catch(() => {
        /* advisory — save_card still dedups by jmdict_id server-side */
      });
    return () => { cancelled = true; };
  }, [live, server.client, entries]);

  // A web IME sends every step of a word it is composing ("t", "た", "たb",
  // "たべ") through onChangeText. While composing, a lookup waits for
  // compositionend or a long pause, so a word costs one request and the
  // results don't flicker mid-word. Only a pause, not a block: Android
  // keyboards compose plain English words too, and commit them late.
  const inputRef = useRef<TextInput>(null);
  const [composing, setComposing] = useState(false);
  useEffect(() => {
    const node = inputRef.current as unknown as HTMLElement | null;
    if (Platform.OS !== 'web' || typeof node?.addEventListener !== 'function') return;
    const start = () => setComposing(true);
    const end = () => setComposing(false);
    node.addEventListener('compositionstart', start);
    node.addEventListener('compositionend', end);
    return () => {
      node.removeEventListener('compositionstart', start);
      node.removeEventListener('compositionend', end);
    };
  }, []);

  // Debounce + stale-response guard, same pattern as VocabScreen.
  const reqSeq = useRef(0);

  useEffect(() => {
    // JP mode converts romaji→kana and looks up by reading/kanji; EN mode
    // reverse-searches by meaning and never touches the raw text.
    const { queries, kana } = mode === 'jp' ? buildQueries(query) : { queries: [], kana: null };
    // Plain letters in JP mode may be English ("study") as easily as romaji
    // ("made" is まで), so meanings are searched too, listed after the
    // Japanese hits.
    const english = mode === 'jp' ? englishQuery(query) : null;
    setSearchedKana(mode === 'jp' ? kana : null);
    const trimmed = query.trim();
    const hasQuery = mode === 'jp' ? queries.length > 0 : trimmed.length > 0;
    if (!live || !hasQuery) {
      setEntries(null);
      setEnglishFrom(null);
      setError(null);
      setLoading(false);
      return;
    }
    const seq = ++reqSeq.current;
    setLoading(true);
    const t = setTimeout(() => {
      const work: Promise<{ entries: DictEntry[]; englishFrom: number | null }> =
        mode === 'jp'
          ? Promise.all([
              server.client!.lookup(queries).then((results) => {
                // Merge entries across queries, deduped by id (order preserved).
                const seen = new Set<number>();
                const merged: DictEntry[] = [];
                for (const r of results) {
                  for (const e of r.entries) {
                    if (seen.has(e.id)) continue;
                    seen.add(e.id);
                    merged.push(e);
                  }
                }
                return orderEntries(merged, settings.definitionLang);
              }),
              // The meaning search is extra; the lookup's result stands alone.
              english ? server.client!.searchEnglish(english).catch(() => [] as DictEntry[]) : Promise.resolve([]),
            ]).then(([japanese, byMeaning]) => {
              const seen = new Set(japanese.map((e) => e.id));
              const extra = byMeaning.filter((e) => !seen.has(e.id));
              return {
                entries: [...japanese, ...extra],
                englishFrom: extra.length > 0 ? japanese.length : null,
              };
            })
          : server.client!.searchEnglish(trimmed).then((found) => ({ entries: found, englishFrom: null }));
      work
        .then((result) => {
          if (reqSeq.current !== seq) return;
          setEntries(result.entries);
          setEnglishFrom(result.englishFrom);
          setError(null);
        })
        .catch((err) => {
          if (reqSeq.current !== seq) return;
          setError(errorText(err));
        })
        .finally(() => {
          if (reqSeq.current === seq) setLoading(false);
        });
    }, composing ? 1200 : 300);
    return () => clearTimeout(t);
  }, [live, server.client, query, mode, composing]);

  const toggleExamples = (e: DictEntry) => {
    if (expanded === e.id) {
      setExpanded(null);
      return;
    }
    setExpanded(e.id);
    if (examples[e.id] || !live) return;
    setExamplesLoading(e.id);
    setExamplesError((prev) => ({ ...prev, [e.id]: false }));
    server
      .client!.examples([headword(e)], 3)
      .then((ex) => setExamples((prev) => ({ ...prev, [e.id]: ex })))
      .catch(() => setExamplesError((prev) => ({ ...prev, [e.id]: true })))
      .finally(() => setExamplesLoading((cur) => (cur === e.id ? null : cur)));
  };

  const save = async (e: DictEntry) => {
    if (!live || saved[e.id] != null) return;
    const client = server.client!;
    setSaving(e.id);
    try {
      const resp = await client.saveCard({
        jmdict_id: e.id,
        lemma: headword(e),
        kanji: e.kanji,
        readings: e.readings,
        glosses: e.senses.map((s) => s.glosses ?? []),
      });
      // Match the Suggested flow: a newly saved card is being studied.
      await client.setWordStatus(headword(e), 1).catch(() => {});
      if (clientRef.current !== client) return;
      setSaved((prev) => ({ ...prev, [e.id]: resp.id }));
      setLastSaved({ id: e.id, word: headword(e) });
      invalidate('vocab', 'stats');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSaving((cur) => (cur === e.id ? null : cur));
    }
  };

  const removeCard = async (e: DictEntry) => {
    const cardId = saved[e.id];
    if (!live || cardId == null) return;
    const client = server.client!;
    setSaving(e.id);
    try {
      await client.deleteCard(cardId);
      if (clientRef.current !== client) return;
      setSaved((prev) => {
        const next = { ...prev };
        delete next[e.id];
        return next;
      });
      invalidate('vocab', 'stats');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setSaving((cur) => (cur === e.id ? null : cur));
    }
  };

  const subtitle = live
    ? entries
      ? `${entries.length} ${entries.length === 1 ? 'entry' : 'entries'}`
      : 'JMdict'
    : 'Connect to a server to search';

  const emptyText = !live
    ? 'Offline. The dictionary lives on your Yume server.'
    : loading
      ? 'Searching…'
      : error
        ? null
        : `No entries for “${query.trim()}”`;

  return (
    <View style={styles.root}>
      <ScreenHeader title="Dictionary" subtitle={subtitle} />
      <DictionaryMissingNotice style={styles.notice} />

      <View style={styles.controls}>
        <View style={styles.modeRow}>
          {(['jp', 'en'] as const).map((m) => {
            const active = mode === m;
            return (
              <Pressable
                key={m}
                onPress={() => setMode(m)}
                style={[styles.modeSeg, active && styles.modeSegActive]}
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
              >
                <Text style={[styles.modeLabel, active && styles.modeLabelActive]}>
                  {m === 'jp' ? '日本語 → EN' : 'EN → 日本語'}
                </Text>
              </Pressable>
            );
          })}
        </View>
        <TextInput
          ref={inputRef}
          style={[styles.search, !live && styles.searchDisabled]}
          placeholder={mode === 'jp' ? 'Japanese or romaji' : 'Search in English'}
          accessibilityLabel="Search dictionary"
          placeholderTextColor={colors.textTertiary}
          value={query}
          onChangeText={setQuery}
          autoCorrect={false}
          autoCapitalize="none"
          returnKeyType="search"
          clearButtonMode="while-editing"
          editable={live}
        />
        {searchedKana ? (
          <Text style={styles.kanaHint}>
            Searching <Text style={styles.kanaHintValue}>{searchedKana}</Text>
          </Text>
        ) : null}
      </View>

      {error && !dictMissing ? <Text style={styles.error}>{error}</Text> : null}
      {lastSaved && saved[lastSaved.id] != null && onReview ? (
        <View style={styles.handoff}>
          <Text style={styles.handoffText} accessibilityLiveRegion="polite">{lastSaved.word} is in your library.</Text>
          <Pressable onPress={onReview} style={styles.actionBtn} accessibilityRole="button">
            <Text style={styles.actionText}>Go to Review</Text>
          </Pressable>
        </View>
      ) : null}

      <FlatList
        data={entries ?? []}
        keyExtractor={(e) => String(e.id)}
        contentContainerStyle={styles.list}
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        keyboardShouldPersistTaps="handled"
        ListEmptyComponent={
          dictMissing ? null : !live || loading || query.trim() ? (
            emptyText ? <Text style={styles.empty}>{emptyText}</Text> : null
          ) : (
            <View style={styles.searchIntro}>
              <View style={styles.actions}>
                {(mode === 'jp'
                  ? (suggested.length > 0 ? suggested.map((w) => w.lemma) : ['夢', '忘れる', '覚える', '三日坊主'])
                  : ['dream', 'forget', 'remember', 'give up']
                ).map((word) => (
                  <Pressable key={word} style={styles.actionBtn} onPress={() => setQuery(word)} accessibilityRole="button">
                    <Text style={styles.actionText}>{word}</Text>
                  </Pressable>
                ))}
              </View>
            </View>
          )
        }
        ListFooterComponent={entries && entries.length > 0 ? <Text style={styles.credit}>{JMDICT_CREDIT}</Text> : null}
        renderItem={({ item, index }) => (
          <>
            {index === englishFrom ? <Text style={styles.sectionLabel}>English meaning matches</Text> : null}
            <DictionaryEntryCard
              entry={item}
              expanded={expanded === item.id}
              examples={examples[item.id]}
              examplesLoading={examplesLoading === item.id}
              examplesFailed={examplesError[item.id] === true}
              saved={saved[item.id] != null}
              saving={saving === item.id}
              onToggleExamples={() => toggleExamples(item)}
              onSave={() => save(item)}
              onRemove={() => setRemoveTarget(item)}
            />
          </>
        )}
      />

      <ConfirmDialog
        visible={removeTarget != null}
        title="Remove from deck?"
        message={
          removeTarget
            ? `Deletes the “${headword(removeTarget)}” card, notes, and media. Review history is kept. Yume will still remember whether you marked the word as learning or known.`
            : undefined
        }
        confirmLabel="Remove"
        onConfirm={() => {
          const target = removeTarget;
          setRemoveTarget(null);
          if (target) void removeCard(target);
        }}
        onCancel={() => setRemoveTarget(null)}
      />
    </View>
  );
}
