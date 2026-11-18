import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AccessibilityInfo,
  Animated,
  Modal,
  Pressable,
  ScrollView,
  Text,
  useWindowDimensions,
  View,
} from 'react-native';
import type { CardNote, DueCard, ExampleSentence, MediaBlob } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { CardHelpSheet } from '../../components/CardHelpSheet';
import { Icon, type IconName } from '../../components/Icon';
import { prefetchSentence } from '../../components/JapaneseSentence';
import { ScreenHeader } from '../../components/ScreenHeader';
import { invalidateIfStale, STATS_STALE_MS } from '../../data/queryStore';
import { unlockAudio } from '../../lib/cardAudio';
import { moonPhaseIcon } from '../../lib/format';
import { useServer } from '../../server/ServerContext';
import { useSettings, useTextScale } from '../../settings/SettingsContext';
import { colors } from '../../theme';
import { useReviewSession } from '../useReviewSession';
import { FadeIn } from './anim';
import { CardFace } from './CardFace';
import { ReviewDone } from './ReviewDone';
import { ReviewIdle } from './ReviewIdle';
import { ReviewFooter } from './ReviewFooter';
import { haptic } from './shared';
import {
  addCachedNote,
  clearAccountContent,
  clearSessionContent,
  loadCardImage,
  loadExamples,
  loadJapaneseDefinition,
  loadNotes,
  peekNotes,
  prefetchReviewCard,
} from './reviewContent';
import { useCardAudio } from './useCardAudio';
import { useCardEdits } from './useCardEdits';
import { useCardTags } from './useCardTags';
import { useReviewKeyboard } from './useReviewKeyboard';
import { useSeenNotes } from './useSeenNotes';
import { styles } from './ReviewScreen.styles';

/**
 * The Review tab: session orchestration — fetching, prefetch caches, audio,
 * keyboard shortcuts, and the in-session chrome. Sub-screens: ReviewIdle,
 * ReviewDone, CardFace. Session STATE lives in useReviewSession.
 */
export default function ReviewScreen({ active = true, onFindWord }: { active?: boolean; onFindWord?: () => void }) {
  const server = useServer();
  const { settings } = useSettings();
  const jpScale = useTextScale();
  const { width: winWidth } = useWindowDimensions();
  const session = useReviewSession();
  const {
    live, mode, phase, card, cardId, currentDue, nextDue, sessionTotal, doneCount, againCount,
    error, notice, newPending, offlineCount, submitting, pendingRating, canUndo, undoDepth,
    lastRated, rate, undo, removeCard, patchCard, reveal, endSession,
  } = session;
  const offlineReady = !live && offlineCount > 0;
  const learnCount = Math.min(settings.newPerLearn, newPending);
  // Card-back content keys off the SESSION mode, not the probe status: the
  // session survives status flaps, so its render surfaces must too.
  const cardLive = mode === 'live' && server.client != null;

  const [removeTarget, setRemoveTarget] = useState<DueCard | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  // The help sheet writes to the card showing. A rating sent before it
  // opened can still land and move the queue on; close the sheet then, so a
  // draft started on one card is never saved onto the next.
  useEffect(() => {
    setHelpOpen(false);
  }, [cardId]);
  // Media ids overlap across accounts, so caches die with the client identity.
  useEffect(() => {
    clearAccountContent();
  }, [server.client]);
  // Notes and clip lists are dropped between sittings so tutor notes and
  // regenerated audio surface next time. Bytes stay (keyed by media id).
  useEffect(() => {
    if (phase === 'idle') clearSessionContent();
  }, [phase]);
  const [examples, setExamples] = useState<ExampleSentence[]>([]);
  const [notes, setNotes] = useState<CardNote[]>([]);
  const { seenNotes, markNoteSeen } = useSeenNotes();

  useEffect(() => {
    if (!lastRated?.grad) return;
    if (lastRated.grad === 'promoted') haptic('success');
    AccessibilityInfo.announceForAccessibility(
      `${lastRated.lemma} ${lastRated.grad === 'promoted' ? 'graduated to Known' : 'back to Learning'}`,
    );
  }, [lastRated]);

  useEffect(() => {
    if (error) AccessibilityInfo.announceForAccessibility(error);
  }, [error]);

  // Async results (audio fetches) are dropped if the queue advanced past their card.
  const currentCardIdRef = useRef<number | null>(null);
  useEffect(() => {
    currentCardIdRef.current = card?.id ?? null;
  });
  // Late-resolving fetches compare against this before caching, so one
  // account's data never lands in caches just cleared for the next.
  const clientRef = useRef(server.client);
  useEffect(() => {
    clientRef.current = server.client;
  });

  const { audioClips, playWord, playClip, soundBlocked, enableSound } = useCardAudio({
    active,
    live,
    client: server.client,
    phase,
    card,
    cardAudio: settings.cardAudio,
    autoPlayAudio: settings.autoPlayAudio,
    currentCardIdRef,
    clientRef,
  });

  const [cardImage, setCardImage] = useState<MediaBlob | null>(null);
  const cardImagePath = card?.imagePath;
  useEffect(() => {
    setCardImage(null);
    if ((phase !== 'front' && phase !== 'back') || !cardLive || !server.client) return;
    if (!cardId || cardId < 0 || !cardImagePath) return;
    const id = cardId;
    const client = server.client;
    let cancelled = false;
    loadCardImage(client, id, () => clientRef.current === client)
      .then((image) => {
        if (!cancelled && currentCardIdRef.current === id) setCardImage(image);
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [phase, cardLive, server.client, cardId, cardImagePath]);

  useEffect(() => {
    if (active) invalidateIfStale(STATS_STALE_MS, 'stats');
  }, [active]);

  // Corpus examples (with English) — live cards only store the Japanese sentence.
  const cardLemma = card?.lemma;
  const cardReading = card?.reading;
  useEffect(() => {
    if (!cardLive || !cardLemma || !settings.showExamples) {
      setExamples([]);
      return;
    }
    let cancelled = false;
    setExamples([]);
    const client = server.client!;
    loadExamples(client, cardLemma, cardReading)
      .then((clean) => {
        if (cancelled) return;
        setExamples(clean);
      })
      .catch(() => { if (!cancelled) setExamples([]); });
    return () => { cancelled = true; };
  }, [cardLive, cardLemma, cardReading, server.client, settings.showExamples]);

  // Japanese definition, as the meaning or as the second line under it.
  const [jaDefinition, setJaDefinition] = useState<string | null>(null);
  const wantJa = settings.definitionLang === 'ja' || settings.secondDefinition;
  useEffect(() => {
    setJaDefinition(null);
    if (!cardLive || !server.client || !cardLemma || !wantJa) return;
    let cancelled = false;
    loadJapaneseDefinition(server.client, cardLemma)
      .then((d) => { if (!cancelled) setJaDefinition(d); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [cardLive, cardLemma, server.client, wantJa]);

  const cardSentence = card?.sentence;
  useEffect(() => {
    if (cardLive && server.client && cardSentence) {
      prefetchSentence(server.client, cardSentence);
    }
  }, [cardLive, server.client, cardSentence]);

  const nextId = nextDue?.id;
  useEffect(() => {
    if (!cardLive || !server.client || !nextDue) return;
    const client = server.client;
    prefetchReviewCard(
      client,
      nextDue,
      settings.showExamples,
      () => clientRef.current === client,
      wantJa,
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cardLive, server.client, nextId, settings.showExamples, wantJa]);

  useEffect(() => {
    if (!cardLive || !cardId || cardId < 0) { setNotes([]); return; }
    let cancelled = false;
    const cached = peekNotes(cardId);
    setNotes(cached ?? []);
    if (cached) return;
    const client = server.client!;
    loadNotes(client, cardId, () => clientRef.current === client)
      .then((n) => {
        if (cancelled) return;
        setNotes(n);
      })
      .catch(() => { if (!cancelled) setNotes([]); });
    return () => { cancelled = true; };
  }, [cardLive, cardId, server.client]);

  // Active-recall: English hidden until tapped, reset per card. The counters
  // are deps too: with again-repeat and ONE card left, the same id comes
  // straight back.
  const [showEnglish, setShowEnglish] = useState(false);
  useEffect(() => { setShowEnglish(false); }, [cardId, doneCount, againCount]);

  // Reveal is a tap (or key press): unlock sound there so the auto-play that
  // follows the flip is allowed on iPhone Safari.
  const revealWithSound = useCallback(() => {
    unlockAudio();
    reveal();
  }, [reveal]);

  useReviewKeyboard({
    active,
    phase,
    submitting,
    canUndo,
    blocked: removeTarget != null || menuOpen || helpOpen,
    rate,
    undo,
    reveal: revealWithSound,
  });

  const confirmRemove = () => {
    if (!live || submitting) return;
    if (!currentDue || currentDue.id < 0) return;
    setRemoveTarget(currentDue);
  };

  const { isCardTagged, toggleCardTag, savingTag: helpSavingTag, error: helpError, clearError: clearHelpError } =
    useCardTags({ live, client: server.client, currentDue });

  const { saveEdit, saving: editSaving, error: editError, clearError: clearEditError } = useCardEdits({
    live,
    client: server.client,
    currentDue,
    patchCard,
    onNoteAdded: (note) => {
      addCachedNote(note.card_id, note);
      if (currentCardIdRef.current === note.card_id) setNotes((prev) => [note, ...prev]);
    },
  });

  const sessionSubtitle =
    mode === 'sample'
      ? `${doneCount + 1} of ${sessionTotal} · sample deck`
      : mode === 'offline'
        ? `${doneCount + 1} of ${sessionTotal} · offline`
        : `${doneCount + 1} of ${sessionTotal}`;

  const progressPct = sessionTotal > 0 ? (doneCount / sessionTotal) * 100 : 0;
  const progressAnim = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.timing(progressAnim, {
      toValue: progressPct,
      duration: 200,
      useNativeDriver: false,
    }).start();
  }, [progressAnim, progressPct]);

  if (phase === 'idle' || phase === 'loading') {
    return <ReviewIdle session={session} learnCount={learnCount} offlineReady={offlineReady} onFindWord={onFindWord} />;
  }

  if (phase === 'done' || !card) {
    return <ReviewDone session={session} learnCount={learnCount} />;
  }

  // English glosses count only when the card shows them: with a Japanese
  // definition as the meaning, they are a second line the learner can opt in to.
  const jaPrimary = settings.definitionLang === 'ja' && !!jaDefinition;
  const englishLine = !jaPrimary || settings.secondDefinition;
  const hasEnglish = (englishLine && !!card.glosses) || !!card.sentenceTranslation || !!card.explanation;
  const englishHidden = settings.englishReveal === 'tap' && !showEnglish;

  const flagTagged = isCardTagged('flagged', card.id);
  const furigana = isCardTagged('furigana', card.id);

  // An example note is the PRIMARY sentence only when the card has none of
  // its own (the server folds those in at migration; this is the fallback);
  // otherwise it joins the Examples section.
  const sentenceNotes = notes.filter((n) => n.kind === 'example');
  const primarySentenceNote = !card.sentence && sentenceNotes.length > 0 ? sentenceNotes[0] : null;
  const noteExamples = sentenceNotes
    .filter((n) => n.id !== primarySentenceNote?.id)
    .map((n) => ({ text: n.note, translation: n.translation ?? '' }));
  const exampleRows = [...noteExamples, ...examples];


  return (
    <View style={styles.root}>
      <View style={styles.headerRow}>
        <View style={styles.headerFill}>
          {/* Mid-session the count is the title; "Review" is the tab, not news. */}
          <ScreenHeader title={sessionSubtitle} />
        </View>
        {live ? (
          <Pressable
            style={({ pressed }) => [styles.iconBtn, pressed && styles.pressed]}
            onPress={() => setMenuOpen(true)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel="Card actions (tutor, flag, remove)"
          >
            <Icon name="dots-vertical" size={18} color={colors.textSecondary} />
          </Pressable>
        ) : null}
        <Pressable
          style={({ pressed }) => [styles.endBtn, pressed && styles.pressed]}
          onPress={endSession}
          hitSlop={8}
          accessibilityRole="button"
          accessibilityLabel="End session and return to start"
        >
          <Icon name="close" size={14} color={colors.textTertiary} />
          <Text style={styles.endLabel}>End</Text>
        </Pressable>
      </View>

      <Modal visible={menuOpen} transparent animationType="fade" onRequestClose={() => setMenuOpen(false)}>
        <Pressable style={styles.menuBackdrop} onPress={() => setMenuOpen(false)}>
          <View style={styles.menuCard}>
            {canUndo ? (
              <MenuRow
                icon="undo-variant"
                label={undoDepth > 1 ? `Undo last rating (${undoDepth})` : 'Undo last rating'}
                onPress={() => { setMenuOpen(false); haptic('tap'); void undo(); }}
              />
            ) : null}
            <MenuRow
              icon="robot-happy-outline"
              label="Improve this card"
              active={['needs-explanation', 'needs-sentence', 'needs-audio', 'needs-fix'].some((tag) => isCardTagged(tag, card.id))}
              onPress={() => { setMenuOpen(false); clearHelpError(); clearEditError(); setHelpOpen(true); }}
            />
            <MenuRow
              icon="eye-outline"
              label={furigana ? 'Hide furigana on the front' : 'Show furigana on the front'}
              active={furigana}
              onPress={() => { setMenuOpen(false); void toggleCardTag('furigana'); }}
            />
            <MenuRow
              icon="flag-variant-outline"
              label={flagTagged ? 'Unflag this card' : 'Flag a problem'}
              active={flagTagged}
              onPress={() => { setMenuOpen(false); void toggleCardTag('flagged'); }}
            />
            <MenuRow
              icon="trash-can-outline"
              label="Remove card from deck"
              destructive
              onPress={() => { setMenuOpen(false); confirmRemove(); }}
            />
          </View>
        </Pressable>
      </Modal>

      <CardHelpSheet
        visible={helpOpen}
        lemma={card.lemma}
        isRequested={(tag) => isCardTagged(tag, card.id)}
        savingTag={helpSavingTag}
        error={helpError}
        onToggle={(tag) => { void toggleCardTag(tag); }}
        onClose={() => setHelpOpen(false)}
        content={{
          sentence: card.sentence ?? '',
          translation: card.sentenceTranslation ?? '',
          explanation: card.explanation ?? '',
          mnemonicParts: card.mnemonic?.parts ?? [],
          mnemonicStory: card.mnemonic?.story ?? '',
          mnemonicShared: currentDue?.mnemonic_shared ?? false,
        }}
        onSaveEdit={saveEdit}
        editSaving={editSaving}
        editError={editError}
        onEditorChange={clearEditError}
      />

      <ConfirmDialog
        visible={removeTarget != null}
        title="Remove this card?"
        message={
          removeTarget
            ? `Deletes the “${removeTarget.lemma}” flashcard for good. The word itself stays in your library.`
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

      <View style={styles.progressRow}>
        <Icon name={moonPhaseIcon(progressPct)} size={14} color={colors.accent} />
        <View style={styles.progressTrack}>
          <Animated.View
            style={[
              styles.progressBar,
              {
                width: progressAnim.interpolate({
                  inputRange: [0, 100],
                  outputRange: ['0%', '100%'],
                }),
              },
            ]}
          />
        </View>
        {/* With again-repeat the progress fraction stalls while a card laps;
            this is where an Again visibly counts. */}
        {againCount > 0 ? (
          <View
            style={styles.lapBadge}
            accessibilityLabel={`${againCount} Again ${againCount === 1 ? 'rating' : 'ratings'} this session`}
          >
            <Icon name="repeat" size={12} color={colors.statusUnknown} />
            <Text style={styles.lapText}>{againCount}</Text>
          </View>
        ) : null}
      </View>

      {soundBlocked ? (
        <Pressable
          style={({ pressed }) => [styles.soundBtn, pressed && styles.pressed]}
          onPress={enableSound}
          accessibilityRole="button"
        >
          <Icon name="volume-high" size={15} color={colors.accentText} />
          <Text style={styles.soundLabel}>Tap to enable sound</Text>
        </Pressable>
      ) : null}

      <ScrollView contentContainerStyle={styles.cardScroll}>
        {/* Re-keyed on every rating; doneCount + againCount cover the
            again-repeat case where the SAME card id comes straight back. */}
        <FadeIn
          key={`${card.id}:${doneCount}:${againCount}`}
          style={styles.cardFade}
          duration={150}
          drift={8}
        >
          <CardFace
            phase={phase}
            card={card}
            jpScale={jpScale}
            winWidth={winWidth}
            live={cardLive}
            client={server.client}
            audioClips={audioClips}
            image={cardImage}
            notes={notes}
            seenNotes={seenNotes}
            markNoteSeen={markNoteSeen}
            englishHidden={englishHidden}
            hasEnglish={hasEnglish}
            jaDefinition={jaDefinition}
            definitionLang={settings.definitionLang}
            secondDefinition={settings.secondDefinition}
            collapseMnemonics={settings.collapseMnemonics}
            markNewParts={settings.markNewParts}
            onShowEnglish={() => setShowEnglish(true)}
            onReveal={revealWithSound}
            playWord={playWord}
            playClip={(m) => void playClip(m)}
            primarySentenceNoteId={primarySentenceNote?.id ?? null}
            exampleRows={exampleRows}
            corpusExamples={examples.length > 0}
            furigana={furigana}
            onToggleFurigana={live ? () => { void toggleCardTag('furigana'); } : undefined}
          />
        </FadeIn>
      </ScrollView>

      <ReviewFooter
        phase={phase}
        card={card}
        mode={mode}
        error={error}
        notice={notice}
        lastRated={lastRated}
        canUndo={canUndo}
        submitting={submitting}
        pendingRating={pendingRating}
        againRepeats={settings.againRepeats}
        rate={rate}
        undo={undo}
        reveal={revealWithSound}
      />
    </View>
  );
}

function MenuRow({
  icon,
  label,
  onPress,
  active = false,
  destructive = false,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
  active?: boolean;
  destructive?: boolean;
}) {
  const tint = destructive ? colors.statusUnknown : active ? colors.accentText : colors.textSecondary;
  return (
    <Pressable
      style={({ pressed }) => [styles.menuRow, pressed && styles.menuRowPressed]}
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
    >
      <Icon name={icon} size={17} color={tint} />
      <Text style={[styles.menuLabel, { color: tint }]}>{label}</Text>
    </Pressable>
  );
}
