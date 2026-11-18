import { useCallback, useEffect, useRef, useState, type RefObject } from 'react';
import type { YumeClient } from '../../api/client';
import type { CardMediaInfo } from '../../api/types';
import type { ReviewCardVM } from '../../data/live';
import { isPlaybackBlocked, playCardAudio, stopCardAudio, unlockAudio } from '../../lib/cardAudio';
import { speak, stopSpeaking } from '../../lib/speech';
import type { CardAudioMode } from '../../settings/SettingsContext';
import type { Phase } from '../useReviewSession';
import {
  loadAudioBytes,
  loadPrimaryMedia,
  peekAudioBytes,
  peekPrimaryMedia,
  prefetchCardAudio,
} from './reviewContent';

/** Card audio: generated clips, TTS fallback, auto-play on reveal, and
 *  stopping playback when the tab hides or the queue advances.
 *
 *  Clips are fetched while the card is face-down and played from cache with
 *  no await in between, so a tap's play() stays inside the tap (iPhone
 *  Safari blocks it otherwise). If the browser still blocks playback,
 *  `soundBlocked` asks for one tap, once. */
export function useCardAudio({
  active,
  live,
  client,
  phase,
  card,
  cardAudio,
  autoPlayAudio,
  currentCardIdRef,
  clientRef,
}: {
  active: boolean;
  live: boolean;
  client: YumeClient | null;
  phase: Phase;
  card: ReviewCardVM | null;
  cardAudio: CardAudioMode;
  autoPlayAudio: boolean;
  /** Async results are dropped if the queue advanced past their card. */
  currentCardIdRef: RefObject<number | null>;
  /** Late fetches compare against this so one account's data never lands
   *  in caches just cleared for the next. */
  clientRef: RefObject<YumeClient | null>;
}): {
  audioClips: CardMediaInfo[];
  playWord: (c: ReviewCardVM) => void;
  playClip: (m: CardMediaInfo) => Promise<void>;
  /** The browser refused to play without a tap; offer one. */
  soundBlocked: boolean;
  /** For the "Tap to enable sound" button: unlock, then replay the card. */
  enableSound: () => void;
} {
  const cardId = card?.id;

  const [soundBlocked, setSoundBlocked] = useState(false);
  const blockedOnceRef = useRef(false);
  // Audio is a nicety: failures never block review. A browser block is the
  // one failure worth a word, and only the first time.
  const play = useCallback((mime: string, data: string) => {
    playCardAudio(mime, data).catch((err) => {
      if (isPlaybackBlocked(err) && !blockedOnceRef.current) {
        blockedOnceRef.current = true;
        setSoundBlocked(true);
      }
    });
  }, []);

  const playGenerated = useCallback(
    async (id: number) => {
      if (!live || !client) return;
      // Prefetched: play inside the caller's tap, before any await.
      const cached = peekPrimaryMedia(id);
      if (cached) {
        if (cached.audio?.data) play(cached.audio.mime, cached.audio.data);
        return;
      }
      try {
        const media = await loadPrimaryMedia(client, id, () => clientRef.current === client);
        // A slow fetch can outlive its card; card ids overlap across
        // accounts, so the id check alone can pass after a client switch.
        if (clientRef.current !== client) return;
        if (currentCardIdRef.current !== id) return;
        if (media?.audio?.data) play(media.audio.mime, media.audio.data);
      } catch {
        /* audio is a nicety — never block review on it */
      }
    },
    [live, client, clientRef, currentCardIdRef, play],
  );

  // Clip list fetched while the card is still FACE-DOWN: the chip row sits
  // above the glosses, so a post-reveal arrival would shift the layout.
  const [audioClips, setAudioClips] = useState<CardMediaInfo[]>([]);
  useEffect(() => { setAudioClips([]); }, [cardId]);
  const cardAudioPath = card?.audioPath;
  useEffect(() => {
    if ((phase !== 'front' && phase !== 'back') || !live || !client) return;
    if (!cardId || cardId < 0 || !cardAudioPath) return;
    const id = cardId;
    let cancelled = false;
    prefetchCardAudio(client, id, () => clientRef.current === client)
      .then((audio) => {
        if (cancelled) return;
        if (currentCardIdRef.current === id) setAudioClips(audio);
      })
      .catch(() => { /* chips just don't render — audio is a nicety */ });
    return () => { cancelled = true; };
  }, [phase, live, client, cardId, cardAudioPath, clientRef, currentCardIdRef]);

  const playClip = useCallback(
    async (m: CardMediaInfo) => {
      if (!live || !client) return;
      const cached = peekAudioBytes(m.id);
      if (cached) {
        stopSpeaking();
        play(cached.mime, cached.data);
        return;
      }
      try {
        const blob = await loadAudioBytes(client, m.id, () => clientRef.current === client);
        if (!blob) return;
        if (currentCardIdRef.current !== m.card_id) return;
        stopSpeaking();
        play(blob.mime, blob.data);
      } catch {
        /* audio is a nicety — never block review on it */
      }
    },
    [live, client, clientRef, currentCardIdRef, play],
  );

  const playWord = useCallback(
    (c: ReviewCardVM) => {
      if (cardAudio === 'generated' && live && c.audioPath && c.id >= 0) {
        void playGenerated(c.id);
      } else {
        speak(c.reading || c.lemma);
      }
    },
    [cardAudio, live, playGenerated],
  );

  // Auto-play on reveal, once per card, ONLY while this tab is `active`: every
  // tab stays mounted, so flipping the audio setting elsewhere would
  // otherwise re-run this effect and speak the lingering card.
  const lastPlayedRef = useRef<number | null>(null);
  useEffect(() => {
    if (phase === 'front') {
      lastPlayedRef.current = null;
      return;
    }
    if (phase !== 'back' || !active || !autoPlayAudio) return;
    if (!card || card.id < 0) return;
    if (lastPlayedRef.current === card.id) return;
    lastPlayedRef.current = card.id;
    playWord(card);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase, active, card?.id, card?.audioPath, cardAudio, autoPlayAudio, playWord]);

  useEffect(() => {
    if (!active || phase === 'idle' || phase === 'done') {
      void stopCardAudio();
      stopSpeaking();
    }
  }, [active, phase]);

  // Cut the previous card's audio as the queue advances. Cleanup runs before
  // the new card's auto-play effect, so it can't silence the incoming card.
  useEffect(() => {
    return () => {
      void stopCardAudio();
      stopSpeaking();
    };
  }, [cardId]);

  // Replays only a revealed card: on a face-down card the word's audio
  // would give the answer away.
  const enableSound = useCallback(() => {
    unlockAudio();
    setSoundBlocked(false);
    if (phase === 'back' && card && card.id >= 0) playWord(card);
  }, [phase, card, playWord]);

  return { audioClips, playWord, playClip, soundBlocked, enableSound };
}

