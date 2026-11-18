import { Icon } from '../components/Icon';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Platform,
  Pressable,
  Text,
  TextInput,
  View,
  type TextStyle,
} from 'react-native';
import type { VocabFilter, WordStatusValue } from '../api/types';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { WordPopup } from '../components/WordPopup';
import { vocabRowToCard } from '../data/live';
import { invalidate, useServerQuery } from '../data/queryStore';
import { errorText } from '../lib/errors';
import { SAMPLE_CARDS } from '../data/sample';
import type { Card, WordStatus } from '../data/types';
import { useServer } from '../server/ServerContext';
import { colors, langJa, statusColor } from '../theme';
import { styles } from './VocabScreen.styles';
import { VocabRow } from './VocabRow';

// No filter = everything; tapping the active chip again clears it.
type Filter = WordStatus | 'new';

// Drops the browser's default focus ring (searchFocused takes its job).
// outlineStyle is web-only and absent from RN's TextStyle, hence the cast.
const webNoOutline =
  Platform.OS === 'web' ? ({ outlineStyle: 'none' } as unknown as TextStyle) : null;

const FILTERS: { key: Filter; label: string; wire?: WordStatusValue; color?: string }[] = [
  // Cards added but never reviewed (cards.reps = 0).
  { key: 'new', label: 'New' },
  { key: 'learning', label: 'Learning', wire: 1, color: colors.statusLearning },
  { key: 'known', label: 'Known', wire: 2, color: colors.statusKnown },
  { key: 'unknown', label: 'Unknown', wire: 0, color: colors.statusUnknown },
  { key: 'ignored', label: 'Ignored', wire: 3, color: colors.statusIgnored },
];

// Short enough for one row on a 390pt phone. "Rare" = no frequency rank,
// the same word the dictionary badge uses.
const BANDS: { key: string; label: string }[] = [
  { key: '1k', label: 'Top 1k' },
  { key: '3k', label: 'to 3k' },
  { key: '6k', label: 'to 6k' },
  { key: '12k', label: 'to 12k' },
  { key: 'beyond', label: '12k+' },
  { key: 'unranked', label: 'Rare' },
];

/** Search and status filters applied on the device, for the sample words and
 *  for the words kept while the server is unreachable. The band row is hidden
 *  then, so bands don't filter. */
function filterLocal(cards: Card[], query: string, filter: Filter | null): Card[] {
  const q = query.trim().toLowerCase();
  return cards.filter((c) => {
    if (filter && filter !== 'new' && c.status !== filter) return false;
    if (!q) return true;
    return (
      c.lemma.includes(q) ||
      (c.reading ?? '').includes(q) ||
      c.glosses.some((g) => g.toLowerCase().includes(q)) ||
      (c.sentence ?? '').includes(q) ||
      c.tags.some((t) => t.toLowerCase().includes(q))
    );
  });
}

export default function VocabScreen() {
  const server = useServer();
  // 'checking' counts as live, as in useReviewSession: a re-probe must not
  // swap the library out from under the learner.
  const live = (server.status === 'online' || server.status === 'checking') && server.client != null;
  // Sample words stand in only when no server was ever set up. An unreachable
  // server keeps the words last loaded from it, never strangers' words.
  const sample = server.loaded && !server.url;

  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<Filter | null>(null);
  const [band, setBand] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Card | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Card | null>(null);
  const [editing, setEditing] = useState(false);
  const [searchFocused, setSearchFocused] = useState(false);


  const [debouncedQuery, setDebouncedQuery] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim()), 300);
    return () => clearTimeout(t);
  }, [query]);

  const wireStatus = FILTERS.find((f) => f.key === filter)?.wire;
  // Browsing shows frequency-banded tiles; a search or edit mode shows the
  // list, where glosses and delete buttons live.
  const browsing = live && !debouncedQuery && !editing;
  const scopeFilter: VocabFilter = {
    ...(wireStatus !== undefined ? { status: wireStatus } : {}),
    ...(filter === 'new' ? { new: true } : {}),
  };
  const wireFilter: VocabFilter = {
    ...scopeFilter,
    ...(band ? { band } : {}),
    ...(debouncedQuery ? { search: debouncedQuery } : {}),
    ...(browsing ? { sort: 'frequency' as const } : {}),
  };
  const pageSize = browsing ? 200 : 100;
  // 'checking' no longer blanks the query, so a switch to another server or
  // account refetches through server.client, and each result carries the
  // identity it was fetched for: the previous deck's rows (whose card ids
  // mean other cards here) never show under the new one.
  const identity = server.identity;
  const vocabQuery = useServerQuery(
    'vocab',
    live
      ? () =>
          server.client!
            .listVocab(wireFilter, pageSize, 0)
            .then((resp) => ({
              identity,
              rows: (resp.rows ?? []).map(vocabRowToCard),
              total: resp.total,
            }))
      : null,
    [live, server.client, identity, debouncedQuery, filter, band, browsing],
  );
  const bandsQuery = useServerQuery(
    'vocab',
    live && browsing
      ? () => server.client!.vocabBands(scopeFilter).then((bands) => ({ identity, bands }))
      : null,
    [live, server.client, identity, browsing, filter],
  );
  const vocabData = vocabQuery.data?.identity === identity ? vocabQuery.data : null;
  const bandsData = bandsQuery.data?.identity === identity ? bandsQuery.data.bands : null;
  const liveRows = vocabData?.rows ?? null;
  const liveTotal = vocabData?.total ?? 0;
  const loading = vocabQuery.loading;

  // The last unfiltered page, shown (and searched on the device) while the
  // server is unreachable. Card ids overlap across accounts, so it is dropped
  // on an account switch.
  const [kept, setKept] = useState<Card[] | null>(null);
  useEffect(() => {
    setKept(null);
  }, [server.identity]);
  useEffect(() => {
    if (vocabData && !debouncedQuery && !filter && !band) setKept(vocabData.rows);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vocabData]);

  // Pages past the first 100. Reset whenever page 0 changes identity — after
  // a save/delete/filter refetch, stale extras could show deleted rows.
  const [extraRows, setExtraRows] = useState<Card[]>([]);
  const [loadingMore, setLoadingMore] = useState(false);
  useEffect(() => {
    setExtraRows([]);
  }, [vocabData]);
  const identityRef = useRef(identity);
  identityRef.current = identity;

  const loadMore = () => {
    if (!live || !server.client || liveRows === null || loadingMore) return;
    const have = liveRows.length + extraRows.length;
    if (have >= liveTotal) return;
    setLoadingMore(true);
    server.client
      .listVocab(wireFilter, pageSize, have)
      .then((resp) => {
        if (identityRef.current !== identity) return;
        const next = (resp.rows ?? []).map(vocabRowToCard);
        setExtraRows((prev) => [...prev, ...next]);
      })
      .catch((err) => setError(errorText(err)))
      .finally(() => setLoadingMore(false));
  };

  // Drops the flashcard only; the lemma's word_status survives. Not undoable.
  const requestRemove = (card: Card) => {
    if (!live || !server.client || card.id < 0) return;
    setDeleteTarget(card);
  };

  const performRemove = (card: Card) => {
    setDeletingId(card.id);
    server
      .client!.deleteCard(card.id)
      .then(() => invalidate('vocab', 'stats'))
      .catch((err) => setError(errorText(err)))
      .finally(() => setDeletingId(null));
  };

  const localRows = useMemo(
    () => (live ? [] : filterLocal(sample ? SAMPLE_CARDS : kept ?? [], query, filter)),
    [live, sample, kept, query, filter],
  );

  // Before the stored connection is read, nothing is known yet: not sample,
  // not offline.
  const firstLoad = (live && liveRows === null) || !server.loaded;
  // Dedup across page boundaries: a row can shift pages between fetches, and
  // FlatList throws on duplicate keys.
  const rows = useMemo(() => {
    if (!live) return localRows;
    const base = liveRows ?? [];
    if (extraRows.length === 0) return base;
    const seen = new Set(base.map((c) => `${c.id}:${c.lemma}`));
    return [...base, ...extraRows.filter((c) => !seen.has(`${c.id}:${c.lemma}`))];
  }, [live, liveRows, extraRows, localRows]);
  // Tile sections in server order (most common band first).
  const sections = useMemo(() => {
    if (!browsing) return [];
    const order: string[] = [];
    const byBand = new Map<string, Card[]>();
    for (const c of rows) {
      const key = c.band ?? 'unranked';
      if (!byBand.has(key)) { byBand.set(key, []); order.push(key); }
      byBand.get(key)!.push(c);
    }
    const meta = new Map((bandsData ?? []).map((b) => [b.band, b]));
    return order.map((key) => ({ key, cards: byBand.get(key)!, meta: meta.get(key) }));
  }, [browsing, rows, bandsData]);

  // The count rides in the search field instead of a line of its own.
  const placeholder = live
    ? liveTotal > 0 ? `Search ${liveTotal} ${liveTotal === 1 ? 'word' : 'words'}` : 'Search your words'
    : sample ? `Search ${SAMPLE_CARDS.length} sample words` : 'Search your words';
  // Rows that are not live from the server always say where they came from.
  const notice = live || !server.loaded
    ? null
    : sample
      ? 'Sample words · connect your server in Settings to see your own'
      : kept && kept.length > 0
        ? 'Offline · showing the words loaded before the connection dropped'
        : 'Offline · your words return when the server is reachable';

  return (
    <View style={styles.root}>
      <View style={styles.controls}>
        <View style={styles.searchRow}>
          <TextInput
            style={[styles.search, styles.searchFill, webNoOutline, searchFocused && styles.searchFocused]}
            placeholder={placeholder}
            placeholderTextColor={colors.textTertiary}
            value={query}
            onChangeText={setQuery}
            onFocus={() => setSearchFocused(true)}
            onBlur={() => setSearchFocused(false)}
            autoCorrect={false}
            autoCapitalize="none"
          />
          {live ? (
            <Pressable
              style={[styles.editBtn, editing && styles.editBtnActive]}
              onPress={() => setEditing((e) => !e)}
              accessibilityRole="button"
              accessibilityLabel={editing ? 'Done editing' : 'Edit deck (show delete buttons)'}
              accessibilityState={{ selected: editing }}
            >
              {/* Not pencil-outline: that's the change-request handle's icon. */}
              <Icon
                name={editing ? 'check' : 'playlist-edit'}
                size={18}
                color={editing ? colors.accentText : colors.textSecondary}
              />
            </Pressable>
          ) : null}
        </View>
        <View style={styles.filterRow}>
          {FILTERS.map((f) => {
            const active = filter === f.key;
            const accent = f.color ?? colors.accentText;
            return (
              <Pressable
                key={f.key}
                style={[
                  styles.chip,
                  active && { borderColor: accent, backgroundColor: accent + '22' },
                ]}
                onPress={() => setFilter(active ? null : f.key)}
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
              >
                <Text
                  style={[
                    styles.chipLabel,
                    f.color != null && { color: f.color },
                    active && { color: colors.textPrimary, fontWeight: '600' },
                  ]}
                >
                  {f.label}
                </Text>
              </Pressable>
            );
          })}
          {loading ? <ActivityIndicator size="small" color={colors.textTertiary} /> : null}
        </View>
        {live ? (
          <View style={[styles.filterRow, styles.bandRow]}>
            {BANDS.map((b) => {
              const active = band === b.key;
              return (
                <Pressable
                  key={b.key}
                  style={[styles.chip, styles.bandChip, active && { borderColor: colors.accentText, backgroundColor: colors.accentMuted }]}
                  onPress={() => setBand(active ? null : b.key)}
                  accessibilityRole="button"
                  accessibilityState={{ selected: active }}
                >
                  <Text style={[styles.chipLabel, active && { color: colors.textPrimary, fontWeight: '600' }]}>{b.label}</Text>
                </Pressable>
              );
            })}
          </View>
        ) : null}
        {notice ? (
          <Text style={styles.notice} accessibilityLiveRegion="polite">{notice}</Text>
        ) : null}
      </View>

      {error || vocabQuery.error ? (
        <Text style={styles.error}>{error ?? vocabQuery.error}</Text>
      ) : null}

      {browsing ? (
      <FlatList
        data={sections}
        keyExtractor={(sec) => sec.key}
        contentContainerStyle={styles.list}
        onRefresh={() => {
          vocabQuery.reload();
          void server.refresh();
        }}
        refreshing={loading}
        onEndReached={loadMore}
        onEndReachedThreshold={0.5}
        ListFooterComponent={
          rows.length > 0 && rows.length < liveTotal ? (
            <Text style={styles.footerMore}>
              {loadingMore ? 'Loading more…' : `${rows.length} of ${liveTotal} · scroll for more`}
            </Text>
          ) : null
        }
        ListEmptyComponent={
          <Text style={styles.empty}>
            {loading || firstLoad ? 'Loading…' : 'Your library is empty. Add your first word from Dictionary.'}
          </Text>
        }
        renderItem={({ item: sec }) => (
          <View style={styles.band}>
            <View style={styles.bandHead}>
              <Text style={styles.bandLabel}>{sec.meta?.label ?? BAND_LABELS[sec.key] ?? sec.key}</Text>
              {sec.meta ? (
                <Text style={styles.bandMeta}>
                  {sec.meta.size
                    ? `${sec.meta.known} of ${sec.meta.size.toLocaleString()} known · ${Math.min(100, Math.round((sec.meta.known / sec.meta.size) * 100))}%`
                    : `${sec.meta.known} of ${sec.meta.total} met known`}
                </Text>
              ) : null}
            </View>
            <View style={styles.tiles}>
              {sec.cards.map((c) => (
                <Pressable
                  key={`${c.id}:${c.lemma}`}
                  style={({ pressed }) => [styles.tile, pressed && styles.rowPressed]}
                  onPress={() => setSelected(c)}
                  accessibilityRole="button"
                  accessibilityLabel={`${c.lemma}, ${c.status}`}
                >
                  <View style={[styles.tileDot, { backgroundColor: statusColor(c.status) }]} />
                  <Text {...langJa} style={[styles.tileText, c.status === 'known' && styles.tileTextKnown]}>{c.lemma}</Text>
                </Pressable>
              ))}
            </View>
          </View>
        )}
      />
      ) : (
      <FlatList
        data={rows}
        keyExtractor={(c) => `${c.id}:${c.lemma}`}
        contentContainerStyle={styles.list}
        onRefresh={() => {
          vocabQuery.reload();
          void server.refresh();
        }}
        refreshing={loading}
        // The server pages at 100 (cap 500).
        onEndReached={loadMore}
        onEndReachedThreshold={0.5}
        ListFooterComponent={
          live && rows.length > 0 && rows.length < liveTotal ? (
            <Text style={styles.footerMore}>
              {loadingMore ? 'Loading more…' : `${rows.length} of ${liveTotal} · scroll for more`}
            </Text>
          ) : null
        }
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        ListEmptyComponent={
          <Text style={styles.empty}>
            {loading || firstLoad
              ? 'Loading…'
              : query
                ? `No matches for “${query}”`
                : !live && !sample
                  ? 'Your words will show here when the server is reachable.'
                  : 'Your library is empty. Add your first word from Dictionary.'}
          </Text>
        }
        renderItem={({ item }) => (
          <VocabRow
            card={item}
            onPress={() => setSelected(item)}
            onDelete={editing && live && item.id >= 0 ? () => requestRemove(item) : undefined}
            deleting={deletingId === item.id}
          />
        )}
      />
      )}

      <WordPopup
        visible={selected != null}
        lemma={selected?.lemma ?? null}
        status={selected?.status ?? 'unknown'}
        onClose={() => setSelected(null)}
        onStatusChange={(_lemma, next) => {
          setSelected((s) => (s ? { ...s, status: next } : s));
          invalidate('vocab', 'stats');
        }}
      />

      <ConfirmDialog
        visible={deleteTarget != null}
        title="Delete this card?"
        message={
          deleteTarget
            ? `Deletes the “${deleteTarget.lemma}” flashcard for good. The word itself stays in your library.`
            : undefined
        }
        onConfirm={() => {
          const target = deleteTarget;
          setDeleteTarget(null);
          if (target) performRemove(target);
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </View>
  );
}

const BAND_LABELS: Record<string, string> = {
  '1k': 'Top 1,000',
  '3k': '1,000 to 3,000',
  '6k': '3,000 to 6,000',
  '12k': '6,000 to 12,000',
  beyond: 'Beyond 12,000',
  unranked: 'Rare',
};
