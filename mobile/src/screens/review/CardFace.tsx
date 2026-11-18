import { useEffect, useState } from 'react';
import { Image, Platform, Pressable, Text, View } from 'react-native';
import type { YumeClient } from '../../api/client';
import type { CardMediaInfo, CardNote, MediaBlob, Mnemonic } from '../../api/types';
import { Icon } from '../../components/Icon';
import { JapaneseSentence } from '../../components/JapaneseSentence';
import { KanjiSheet } from '../../components/KanjiSheet';
import { splitColumns, type SplitColumn } from '../../lib/readingSplit';
import { SentenceBlock } from '../../components/SentenceBlock';
import { SpeakButton } from '../../components/SpeakButton';
import type { ReviewCardVM } from '../../data/live';
import {
  orderNotes,
} from '../../lib/cardNotes';
import { useSettings, type DefinitionLang } from '../../settings/SettingsContext';
import { colors, font, langJa, spacing } from '../../theme';
import { styles } from './CardFace.styles';
import { FadeIn } from './anim';
import { CollapsibleNote } from './CollapsibleNote';
import { haptic } from './shared';
import { EXAMPLES_CREDIT } from '../../lib/credits';
import { voiceCreditLine } from '../../lib/voiceCredit';
import { buildClozeParts, clozeTarget } from '../../lib/cloze';

// Key hints only where a physical keyboard is plausible (desktop web). The
// key listeners stay attached regardless; this only gates the hint copy.
export const SHOW_KEY_HINTS =
  Platform.OS === 'web' &&
  typeof window !== 'undefined' &&
  !!window.matchMedia?.('(any-hover: hover) and (any-pointer: fine)').matches;

// react-native-web ignores adjustsFontSizeToFit, so the front lemma must be
// pre-sized. CJK glyphs advance a full em, ASCII roughly half. The 40px floor
// lets very long lemmas wrap (numberOfLines={2}) instead of shrinking.
function fitFontSize(text: string, maxSize: number, availWidth: number): number {
  const ems = [...text].reduce((w, ch) => w + (ch.charCodeAt(0) > 0x2e7f ? 1 : 0.55), 0);
  return Math.max(Math.min(maxSize, Math.floor(availWidth / Math.max(ems, 1))), 40);
}

// cardScroll + card padding per side, plus slack so an exact fit can't wrap.
const noop = () => {};
const LEMMA_CHROME = 2 * (spacing.md + spacing.md) + spacing.sm;

const KANJI_RE = /[㐀-䶿一-鿿々]/;
const RUBY_RATIO = 0.4;

// A column is as wide as its kanji or its furigana, whichever is wider.
function fitRubySize(columns: SplitColumn[], maxSize: number, availWidth: number): number {
  const ems = columns.reduce(
    (w, c) => w + ('kanji' in c ? Math.max(1, [...c.reading].length * RUBY_RATIO) : [...c.kana].length),
    0,
  );
  return Math.max(Math.min(maxSize, Math.floor(availWidth / Math.max(ems, 1))), 32);
}

/**
 * The review card: face-down front or the revealed back. Pure presentation —
 * all fetching/prefetching stays in ReviewScreen.
 */
export function CardFace({
  phase,
  card,
  jpScale,
  winWidth,
  live,
  client,
  audioClips,
  image,
  notes,
  seenNotes,
  markNoteSeen,
  englishHidden,
  hasEnglish,
  jaDefinition = null,
  definitionLang = 'en',
  secondDefinition = false,
  collapseMnemonics = false,
  markNewParts = false,
  onShowEnglish,
  onReveal,
  playWord,
  playClip,
  primarySentenceNoteId,
  exampleRows,
  corpusExamples = false,
  furigana = false,
  onToggleFurigana,
}: {
  phase: 'front' | 'back';
  card: ReviewCardVM;
  jpScale: number;
  winWidth: number;
  live: boolean;
  client: YumeClient | null;
  audioClips: CardMediaInfo[];
  image: MediaBlob | null;
  notes: CardNote[];
  seenNotes: Set<number>;
  markNoteSeen: (id: number) => void;
  englishHidden: boolean;
  hasEnglish: boolean;
  /** Japanese definition from the server's dictionary, when it has one. */
  jaDefinition?: string | null;
  /** Which language is the meaning. Japanese falls back to English when the
   *  dictionary has nothing for this word. */
  definitionLang?: DefinitionLang;
  /** Also show the other language in a quieter line under the meaning. */
  secondDefinition?: boolean;
  /** Mnemonics start folded instead of opening on young cards. */
  collapseMnemonics?: boolean;
  /** Dot the tiles whose part has not appeared in any Known word. */
  markNewParts?: boolean;
  onShowEnglish: () => void;
  onReveal: () => void;
  playWord: (c: ReviewCardVM) => void;
  playClip: (m: CardMediaInfo) => void;
  primarySentenceNoteId: number | null;
  exampleRows: { text: string; translation: string }[];
  /** Some rows come from the Tanaka Corpus, which needs its credit. */
  corpusExamples?: boolean;
  /** Known by sound: the front shows the reading over the kanji. */
  furigana?: boolean;
  onToggleFurigana?: () => void;
}) {
  // A tapped mnemonic tile; only single kanji open the sheet.
  const [kanjiPick, setKanjiPick] = useState<Mnemonic['parts'][number] | null>(null);
  const readingColumns = splitColumns(card.lemma, card.readingParts);
  // Explanation and tutor notes share one collapsed area, closed by default;
  // an unseen tutor note shows a dot on its link until opened once.
  const [teach, setTeach] = useState<'explanation' | 'tutor' | null>(null);
  useEffect(() => { setTeach(null); }, [card.id]);
  const explanation = card.explanation ?? '';
  const tutorNotes = notes.filter((n) => n.kind === 'tutor');
  const tutorUnseen = tutorNotes.some((n) => !seenNotes.has(n.id));
  const teachRow = explanation || tutorNotes.length > 0 ? (
    <View style={styles.teach}>
      <View style={styles.teachLinks}>
        {explanation ? (
          <Pressable onPress={() => setTeach((t) => (t === 'explanation' ? null : 'explanation'))} accessibilityRole="button" hitSlop={8}>
            <Text style={styles.teachLink}>{teach === 'explanation' ? 'Hide explanation' : 'Show explanation'}</Text>
          </Pressable>
        ) : null}
        {tutorNotes.length > 0 ? (
          <Pressable
            onPress={() => {
              setTeach((t) => (t === 'tutor' ? null : 'tutor'));
              tutorNotes.forEach((n) => markNoteSeen(n.id));
            }}
            accessibilityRole="button"
            hitSlop={8}
          >
            <Text style={styles.teachLink}>
              {teach === 'tutor' ? 'Hide tutor note' : 'Show tutor note'}
              {tutorUnseen && teach !== 'tutor' ? <Text style={styles.teachDot}> •</Text> : null}
            </Text>
          </Pressable>
        ) : null}
      </View>
      {teach === 'explanation' ? <Text style={styles.teachText}>{explanation}</Text> : null}
      {teach === 'tutor'
        ? tutorNotes.map((n) => (
            <Text key={n.id} style={styles.teachText}>{n.note}</Text>
          ))
        : null}
    </View>
  ) : null;
  const hasSentenceClip = live && audioClips.some((m) => (m.label || '').toLowerCase() === 'sentence');
  // The meaning is Japanese only when the dictionary has it; otherwise the
  // English glosses stay the meaning and there is no second line to add.
  const jaPrimary = definitionLang === 'ja' && !!jaDefinition;
  const englishLine = !jaPrimary || secondDefinition;
  const jaSecond = !jaPrimary && secondDefinition && !!jaDefinition;
  const { settings } = useSettings();
  const showVoiceCredit = settings.showVoiceCredit;
  // The credited clip: the one that plays first, else any credited clip.
  const creditedClip = audioClips.find((m) => m.is_primary && m.voice) ?? audioClips.find((m) => m.voice);
  const voiceCredit = creditedClip?.voice ? voiceCreditLine(creditedClip.voice, creditedClip.source) : undefined;
  const clozeParts = card.cardType === 'cloze' && card.sentence
    ? buildClozeParts(card.sentence, [card.clozeTarget, card.surface, card.lemma, ...(card.alsoWritten ?? [])])
    : null;
  const clozeHidden = clozeParts ? clozeTarget(clozeParts) : undefined;
  if (phase === 'front') {
    return (
      <Pressable
        style={styles.card}
        onPress={() => { haptic('select'); onReveal(); }}
        accessibilityRole="button"
        accessibilityHint="Tap to reveal the answer"
      >
        {clozeParts ? (
          <Text {...langJa} style={[styles.clozeFront, jpScale !== 1 && { fontSize: Math.round(28 * jpScale), lineHeight: Math.round(44 * jpScale) }]}>
            {clozeParts.map((p, i) =>
              p.kind === 'blank'
                ? <Text key={i} style={styles.clozeBlank}>［　　］</Text>
                : <Text key={i}>{p.text}</Text>,
            )}
          </Text>
        ) : furigana && KANJI_RE.test(card.lemma) && card.reading ? (
          <FuriganaLemma
            lemma={card.lemma}
            reading={card.reading}
            columns={readingColumns}
            maxSize={Math.round(92 * jpScale)}
            availWidth={winWidth - LEMMA_CHROME}
          />
        ) : (
        <Text
          {...langJa}
          style={[styles.lemmaFront, {
            fontSize: fitFontSize(card.lemma, Math.round(92 * jpScale), winWidth - LEMMA_CHROME),
          }]}
          adjustsFontSizeToFit
          numberOfLines={2}
        >
          {card.lemma}
        </Text>
        )}
        <Text style={styles.flipHint}>
          tap to reveal{SHOW_KEY_HINTS ? ' · space' : ''}
        </Text>
      </Pressable>
    );
  }

  // Top-aligned so late-arriving content grows DOWNWARD instead of re-centering.
  return (
    <View style={[styles.card, styles.cardTop]}>
      {card.isNew ? (
        <View style={styles.newBadge}>
          <Text style={styles.newBadgeText}>New word</Text>
        </View>
      ) : null}
      <Text {...langJa} style={[styles.lemma, jpScale !== 1 && { fontSize: Math.round(50 * jpScale) }]}>
        {card.lemma}
      </Text>
      <FadeIn style={styles.answer} duration={120} drift={4}>
        {/* One row: a button per clip when the card has several, else one Listen. */}
        {live && audioClips.length > 1 ? (
          <View style={styles.clipRow}>
            {audioClips.map((m) => {
              const label = (m.label || 'audio').toLowerCase();
              return (
                <Pressable
                  key={m.id}
                  onPress={() => playClip(m)}
                  style={({ pressed }) => [styles.audioBtn, pressed && { opacity: 0.8 }]}
                  accessibilityRole="button"
                  accessibilityLabel={`Play ${label} audio`}
                >
                  <Icon name="volume-high" size={15} color={colors.accentText} />
                  <Text style={styles.audioBtnLabel}>{label}</Text>
                </Pressable>
              );
            })}
          </View>
        ) : card.lemma || card.reading ? (
          <Pressable
            onPress={() => playWord(card)}
            style={({ pressed }) => [styles.audioBtn, pressed && { opacity: 0.8 }]}
            accessibilityRole="button"
            accessibilityLabel="Play pronunciation"
          >
            <Icon name="volume-high" size={15} color={colors.accentText} />
            <Text style={styles.audioBtnLabel}>Listen</Text>
          </Pressable>
        ) : null}
        {live && showVoiceCredit && voiceCredit ? (
          <Text style={styles.voiceNote}>{voiceCredit}</Text>
        ) : null}
        {readingColumns ? (
          // Each kanji over its own share of the reading: the sound belongs to the kanji.
          <View style={styles.splitRow} accessibilityLabel={card.reading}>
            {readingColumns.map((c, i) =>
              'kanji' in c ? (
                <Pressable
                  key={i}
                  style={({ pressed }) => [styles.splitCol, pressed && { opacity: 0.6 }]}
                  onPress={() => setKanjiPick({ kanji: c.kanji, meaning: '' })}
                  accessibilityRole="button"
                  accessibilityLabel={`${c.kanji} ${c.reading}`}
                >
                  <Text {...langJa} style={styles.splitKanji}>{c.kanji}</Text>
                  <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(20 * jpScale) }]}>{c.reading}</Text>
                </Pressable>
              ) : (
                <View key={i} style={styles.splitCol}>
                  <Text {...langJa} style={styles.splitKanji}> </Text>
                  <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(20 * jpScale) }]}>{c.kana}</Text>
                </View>
              ),
            )}
          </View>
        ) : card.reading ? (
          <Text {...langJa} style={[styles.reading, jpScale !== 1 && { fontSize: Math.round(20 * jpScale) }]}>
            {card.reading}
          </Text>
        ) : null}
        {card.alsoWritten?.length ? (
          <Text style={styles.alsoWritten}>also written <Text {...langJa} style={styles.ja}>{card.alsoWritten.join('・')}</Text></Text>
        ) : null}
        {jaPrimary ? (
          <Text {...langJa} style={[styles.glosses, styles.ja, jpScale !== 1 && { fontSize: Math.round(19 * jpScale) }]}>
            {jaDefinition}
          </Text>
        ) : null}
        {englishHidden && hasEnglish ? (
          <Pressable
            onPress={onShowEnglish}
            accessibilityRole="button"
            accessibilityLabel="Show English meaning and translation"
            hitSlop={8}
          >
            <Text style={styles.showEnglish}>Show English</Text>
          </Pressable>
        ) : englishLine && card.glosses ? (
          <Text style={jaPrimary ? styles.glossesSecondary : styles.glosses}>{card.glosses}</Text>
        ) : null}
        {jaSecond ? (
          <Text {...langJa} style={[styles.glossesSecondary, styles.ja, jpScale !== 1 && { fontSize: Math.round(font.md * jpScale) }]}>
            {jaDefinition}
          </Text>
        ) : null}
        {card.sentence ? (
          <SentenceBlock
            sentence={card.sentence}
            translation={card.sentenceTranslation}
            client={live ? client : null}
            highlight={clozeHidden ?? card.lemma}
            hideTranslation={englishHidden}
            speaker={!hasSentenceClip}
          />
        ) : null}
        {image ? (
          <Image
            source={{ uri: `data:${image.mime || 'image/jpeg'};base64,${image.data}` }}
            style={[
              styles.sourceImage,
              { width: Math.max(180, Math.min(520, winWidth - spacing.md * 4)) },
            ]}
            resizeMode="contain"
            accessibilityLabel={`Source image for ${card.lemma}`}
          />
        ) : null}
        {card.mnemonic && (card.mnemonic.story || card.mnemonic.parts.length > 0) ? (
          <View style={styles.notesBlock}>
            <CollapsibleNote
              title="Mnemonic"
              icon="moon-waxing-crescent"
              body={card.mnemonic.story || card.mnemonic.parts.map((b) => b.kanji).join('')}
              startExpanded={!collapseMnemonics && !englishHidden && !card.mature}
              onSeen={noop}
              content={
                <View style={styles.mnemonic}>
                  {card.mnemonic.parts.length > 0 ? (
                    <View style={styles.breakdown}>
                      {card.mnemonic.parts.map((b, i) => {
                        const tappable = live && !!client && [...b.kanji].length === 1;
                        return (
                          <Pressable
                            key={i}
                            style={({ pressed }) => [styles.breakdownPart, pressed && tappable && { opacity: 0.6 }]}
                            onPress={tappable ? () => setKanjiPick(b) : undefined}
                            disabled={!tappable}
                            accessibilityRole={tappable ? 'button' : undefined}
                            accessibilityLabel={tappable ? `About the kanji ${b.kanji}${markNewParts && b.known === false ? ', new to you' : ''}` : undefined}
                          >
                            <Text {...langJa} style={styles.breakdownKanji}>{b.kanji}</Text>
                            <Text style={styles.breakdownMeaning}>{b.meaning}</Text>
                            {markNewParts && b.known === false ? <View style={styles.breakdownNew} accessibilityElementsHidden /> : null}
                          </Pressable>
                        );
                      })}
                    </View>
                  ) : null}
                  {card.mnemonic.story ? <Text style={styles.story}>{card.mnemonic.story}</Text> : null}
                </View>
              }
            />
          </View>
        ) : null}
        {notes.length > 0 ? (
          <View style={styles.notesBlock}>
            {orderNotes(notes).map((n) => {
              if (n.kind === 'example') {
                if (n.id !== primarySentenceNoteId) return null;
                return (
                  <SentenceBlock
                    key={n.id}
                    sentence={n.note}
                    translation={n.translation ?? ''}
                    client={live ? client : null}
                    highlight={card.lemma}
                    hideTranslation={englishHidden}
                  />
                );
              }
              // Tutor notes render inside the teach row; triage is for the tutor, not the learner.
              return null;
            })}
          </View>
        ) : null}
        {teachRow}
        {onToggleFurigana && (furigana || card.tryKanji) ? (
          <View style={styles.furiganaRow}>
            <Text style={styles.furiganaNote}>
              {furigana && card.tryKanji
                ? 'Steady for a month. Try it without furigana?'
                : furigana
                  ? 'Known by sound: the front shows furigana.'
                  : 'Back to kanji only.'}
            </Text>
            <Pressable onPress={onToggleFurigana} accessibilityRole="button" hitSlop={8}>
              <Text style={styles.teachLink}>{furigana ? (card.tryKanji ? 'Try it' : 'Turn off') : 'Undo'}</Text>
            </Pressable>
          </View>
        ) : null}
        {exampleRows.length > 0 ? (
          <View style={styles.examplesBlock}>
            <Text style={styles.examplesHeading}>Examples</Text>
            {exampleRows.map((ex, i) => (
              <View key={i} style={styles.exampleItem}>
                <View style={styles.exampleHead}>
                  <View style={styles.exampleSentence}>
                    {live && client ? (
                      <JapaneseSentence
                        text={ex.text}
                        client={client}
                        size={font.lg}
                        highlight={card.lemma}
                      />
                    ) : (
                      <Text {...langJa} style={styles.exampleText}>{ex.text}</Text>
                    )}
                  </View>
                  <SpeakButton text={ex.text} lang="ja-JP" compact />
                </View>
                {!englishHidden ? (
                  <Text style={styles.exampleTranslation}>{ex.translation}</Text>
                ) : null}
              </View>
            ))}
            {corpusExamples ? <Text style={styles.examplesCredit}>{EXAMPLES_CREDIT}</Text> : null}
          </View>
        ) : null}
      </FadeIn>
      <KanjiSheet
        kanji={kanjiPick?.kanji ?? null}
        meaning={kanjiPick?.meaning}
        client={live ? client : null}
        onClose={() => setKanjiPick(null)}
      />
    </View>
  );
}

/** The front headword with each kanji's reading above it; the whole reading
 *  over the word when it doesn't split (ateji). */
function FuriganaLemma({
  lemma,
  reading,
  columns,
  maxSize,
  availWidth,
}: {
  lemma: string;
  reading: string;
  columns: SplitColumn[] | null;
  maxSize: number;
  availWidth: number;
}) {
  const cols: SplitColumn[] = columns ?? [{ kanji: lemma, reading }];
  const size = columns
    ? fitRubySize(cols, maxSize, availWidth)
    : fitFontSize(lemma, maxSize, availWidth);
  const rubySize = Math.round(size * RUBY_RATIO);
  return (
    <View style={styles.rubyRow} accessibilityLabel={`${lemma}, ${reading}`}>
      {cols.map((c, i) => (
        <View key={i} style={styles.rubyCol}>
          <Text {...langJa} style={[styles.rubyText, { fontSize: rubySize, lineHeight: Math.round(rubySize * 1.3) }]}>
            {'kanji' in c ? c.reading : ' '}
          </Text>
          <Text {...langJa} style={[styles.lemmaFront, { fontSize: size }]}>
            {'kanji' in c ? c.kanji : c.kana}
          </Text>
        </View>
      ))}
    </View>
  );
}
