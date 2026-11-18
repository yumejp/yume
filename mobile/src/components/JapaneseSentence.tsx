import { useEffect, useState, type ReactNode } from 'react';
import { Pressable, Text, View } from 'react-native';
import type { YumeClient } from '../api/client';
import type { Token, WordStatusValue } from '../api/types';
import { statusName } from '../data/live';
import { invalidate } from '../data/queryStore';
import type { WordStatus } from '../data/types';
import { kataToHira } from '../lib/kana';
import { TEXT_SCALE, useSettings } from '../settings/SettingsContext';
import { colors, font, fontJa, langJa } from '../theme';
import { JapaneseDictionaryPopup } from './JapaneseDictionaryPopup';
import { styles } from './JapaneseSentence.styles';


const KANJI = /[㐀-䶿一-龯]/;
const JP = /[぀-ヿ㐀-䶿一-龯]/;

// Kinsoku shori: characters that must not START a line (closing punctuation)
// or END one (opening brackets); they're grouped into a neighbor's wrap unit.
const NO_LINE_START = /^[。、！？!?．，,.…‥:：;；)）」』】〕｝〉》〟’”ー～]+$/;
const NO_LINE_END = /^[(（「『【〔｛〈《〝‘“]+$/;

function isSymbol(pos?: string): boolean {
  return !!pos && (pos.startsWith('記号') || pos.startsWith('補助記号'));
}

/** A word "you don't know" — anything not yet marked Known or Ignored. */
function isUnfamiliar(s: WordStatus): boolean {
  return s === 'unknown' || s === 'learning';
}

const STATUS_VALUE: Record<WordStatus, WordStatusValue> = {
  unknown: 0,
  learning: 1,
  known: 2,
  ignored: 3,
};


/** Tokens + statuses for a sentence, cached by text so the review screen can
 *  prefetch it while the card is face-down (no reflow at reveal). */
interface SentenceData {
  tokens: Token[];
  statuses: Map<string, WordStatus>;
}

const cache = new Map<string, SentenceData>();
const CACHE_MAX = 300; // Map keeps insertion order, so eviction is FIFO
// Statuses are PER-USER but entries are keyed by text alone, so the cache
// dies with the client identity.
let cacheClient: YumeClient | null = null;
function cacheFor(client: YumeClient): Map<string, SentenceData> {
  if (client !== cacheClient) {
    cache.clear();
    cacheClient = client;
  }
  return cache;
}

async function load(client: YumeClient, text: string): Promise<SentenceData> {
  const store = cacheFor(client);
  const hit = store.get(text);
  if (hit) return hit;
  const tokens = await client.tokenize(text);
  const lemmas = [...new Set(tokens.map((t) => t.lemma || t.surface).filter(Boolean))];
  let statuses = new Map<string, WordStatus>();
  let statusesOk = true;
  try {
    const rows = await client.getWordStatuses(lemmas);
    statuses = new Map(rows.map((r) => [r.lemma, statusName(r.status)]));
  } catch {
    statusesOk = false;
  }
  const data: SentenceData = { tokens, statuses };
  // A transient status failure must NOT be cached; serve it degraded once.
  if (statusesOk && cacheFor(client) === store) {
    store.set(text, data);
    if (store.size > CACHE_MAX) {
      const oldest = store.keys().next().value;
      if (oldest !== undefined) store.delete(oldest);
    }
  }
  return data;
}

/** Warm the cache for a sentence ahead of render (fire-and-forget). */
export function prefetchSentence(client: YumeClient, text: string): void {
  if (text) void load(client, text).catch(() => {});
}

interface Props {
  text: string;
  client: YumeClient;
  /** Base glyph size; furigana scales to ~55% of it. */
  size?: number;
  /** Lemma of the word this card teaches — its tokens get a pill highlight. */
  highlight?: string;
  /** Rendered after the last word, on the same wrap line (e.g. a speaker). */
  trailing?: ReactNode;
}

/**
 * A Japanese sentence with furigana over unfamiliar words; every content word
 * is tappable for an inline dictionary popup. Unfamiliar words carry a
 * status-colored underline while glyphs stay body-colored.
 */
export function JapaneseSentence({ text, client, size = font.jp, highlight, trailing }: Props) {
  const { settings } = useSettings();
  const scaled = Math.round(size * TEXT_SCALE[settings.textSize]);
  const [data, setData] = useState<SentenceData | null>(() => cacheFor(client).get(text) ?? null);
  const [selected, setSelected] = useState<Token | null>(null);

  useEffect(() => {
    const hit = cacheFor(client).get(text);
    if (hit) { setData(hit); return; }
    let cancelled = false;
    setData(null);
    load(client, text)
      .then((d) => { if (!cancelled) setData(d); })
      .catch(() => { if (!cancelled) setData({ tokens: [], statuses: new Map() }); });
    return () => { cancelled = true; };
  }, [text, client]);

  // 10px floor: smaller furigana is illegible for kana.
  const furiSize = Math.max(10, Math.round(scaled * 0.55));

  // Raw sentence until tokens arrive, with the furigana row's height reserved
  // so the line doesn't jump when ruby loads in.
  if (data === null || data.tokens.length === 0) {
    return (
      <View {...langJa} style={styles.row}>
        <View style={styles.col}>
          <Text style={[styles.furi, { fontSize: furiSize, lineHeight: furiSize + 2 }]}> </Text>
          <Text style={{ fontFamily: fontJa, fontSize: scaled, lineHeight: scaled + 4, color: colors.textPrimary }}>{text}</Text>
        </View>
      </View>
    );
  }
  const { tokens, statuses } = data;

  const applyStatus = (lemma: string, status: WordStatus) => {
    void client.setWordStatus(lemma, STATUS_VALUE[status])
      .then(() => invalidate('vocab', 'stats'))
      .catch(() => {});
    for (const entry of cache.values()) {
      if (status === 'unknown') entry.statuses.delete(lemma);
      else entry.statuses.set(lemma, status);
    }
    setData((prev) => {
      if (!prev) return prev;
      const next = new Map(prev.statuses);
      if (status === 'unknown') next.delete(lemma);
      else next.set(lemma, status);
      return { tokens: prev.tokens, statuses: next };
    });
  };

  const selectedLemma = selected ? selected.lemma || selected.surface : '';

  // Wrap units (kinsoku shori): closing punctuation joins the preceding
  // token, opening brackets the following one. Indices ride along for keys.
  const units: { token: Token; index: number }[][] = [];
  let carry: { token: Token; index: number }[] = [];
  tokens.forEach((t, i) => {
    if (NO_LINE_END.test(t.surface)) {
      carry.push({ token: t, index: i });
      return;
    }
    if (NO_LINE_START.test(t.surface) && carry.length === 0 && units.length > 0) {
      units[units.length - 1].push({ token: t, index: i });
      return;
    }
    units.push([...carry, { token: t, index: i }]);
    carry = [];
  });
  if (carry.length > 0) units.push(carry);

  return (
    <>
      <View {...langJa} style={styles.row}>
      {units.map((unit) => (
        <View key={unit[0].index} style={styles.unit}>
        {unit.map(({ token: t, index: i }) => {
        const lemma = t.lemma || t.surface;
        const status = statuses.get(lemma) ?? 'unknown';
        const isTarget = !!highlight && (lemma === highlight || t.surface === highlight);
        const tappable = !isSymbol(t.pos) && JP.test(t.surface);
        const furiganaWanted =
          settings.furigana === 'all'
            ? true
            : settings.furigana === 'unknown'
              ? isUnfamiliar(status)
              : false;
        const showFuri =
          tappable && KANJI.test(t.surface) && furiganaWanted && !!t.reading;
        // Status rides an underline, not the glyph color: any untracked word
        // counts as unknown, and whole-word tints drown the target.
        const marked = tappable && status === 'unknown' && !isTarget;
        // Ruby that is wider than its word may bleed a quarter em past each
        // side; only the rest widens the word, so sentences stay tightly set.
        const furiText = showFuri ? kataToHira(t.reading!) : '';
        const baseW = [...t.surface].reduce((w, ch) => w + (ch.charCodeAt(0) > 0x2e7f ? 1 : 0.55), 0) * scaled;
        const furiW = [...furiText].length * furiSize;
        const bleed = Math.min(Math.max(0, furiW - baseW) / 2, scaled * 0.25);
        const furiBand = furiSize + 2;

        const column = (
          <View
            style={[
              styles.col,
              { paddingTop: furiBand, minWidth: furiText ? furiW - 2 * bleed : undefined },
              isTarget && styles.colTarget,
            ]}
          >
            {furiText ? (
              <Text
                style={[
                  styles.furi,
                  // 4px slack each side: the width is estimated, and a clipped
                  // or wrapped reading is worse than a slightly wide one.
                  { fontSize: furiSize, lineHeight: furiBand, position: 'absolute', top: 0, left: -bleed - 4, right: -bleed - 4 },
                ]}
              >
                {furiText}
              </Text>
            ) : null}
            <Text
              style={{
                fontFamily: fontJa,
                fontSize: scaled,
                lineHeight: scaled + 4,
                color: isTarget ? colors.accentText : colors.textPrimary,
                fontWeight: isTarget ? '700' : '400',
                ...(marked
                  ? {
                      textDecorationLine: 'underline' as const,
                      textDecorationStyle: 'solid' as const,
                      // Half-strength: a status hint under the word, not a mark on it.
                      textDecorationColor: colors.statusUnknown + '80',
                    }
                  : null),
              }}
            >
              {t.surface}
            </Text>
          </View>
        );

        if (!tappable) return <View key={i}>{column}</View>;
        return (
          <Pressable
            key={i}
            onPress={() => setSelected(t)}
            accessibilityRole="button"
            accessibilityLabel={`Define ${t.surface}`}
          >
            {column}
          </Pressable>
        );
        })}
        </View>
      ))}
      {trailing ? <View style={styles.trailing}>{trailing}</View> : null}
      </View>

      <JapaneseDictionaryPopup
        token={selected}
        client={client}
        status={selected ? (statuses.get(selectedLemma) ?? 'unknown') : 'unknown'}
        onSetStatus={(s) => applyStatus(selectedLemma, s)}
        onClose={() => setSelected(null)}
      />
    </>
  );
}
