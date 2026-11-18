import { Platform } from 'react-native';
import * as Speech from 'expo-speech';
import { stripMarkup } from './text';

interface SpeakOpts {
  /** BCP-47 language tag for the voice pick. Defaults to Japanese. */
  lang?: string;
  /** Speaking rate. ~0.95 = mildly slower than native, good for learners. */
  rate?: number;
  /** Called exactly once when the utterance finishes, errors, is stopped, or
   *  never starts (no TTS / empty text) — safe to drive a speaking indicator. */
  onDone?: () => void;
}

/**
 * Speak `text` aloud with the platform's free text-to-speech engine.
 * - Web (Safari/Chrome): the Web Speech API (`speechSynthesis`).
 * - Native (Expo Go / device): `expo-speech`.
 * Any in-flight utterance is cancelled first so calls don't pile up.
 * A no-op when TTS isn't available (e.g. an old web view without the API).
 */
export function speak(rawText: string, opts: SpeakOpts = {}): void {
  const { lang = 'ja-JP', rate = 0.95, onDone } = opts;
  // Guarantee onDone fires exactly once no matter which path/event ends it.
  let done = false;
  const finish = () => {
    if (done) return;
    done = true;
    onDone?.();
  };
  // Never read markup aloud (a mined "<br>" otherwise gets pronounced "br").
  const text = stripMarkup(rawText);
  if (!text) {
    finish();
    return;
  }

  if (Platform.OS === 'web') {
    const W = globalThis as any;
    if (!W?.speechSynthesis || !W?.SpeechSynthesisUtterance) {
      finish();
      return;
    }
    W.speechSynthesis.cancel();
    const u = new W.SpeechSynthesisUtterance(text);
    u.lang = lang;
    u.rate = rate;
    // Hold the reference until it finishes so Chrome can't GC it mid-speech.
    u.onend = () => { if (webUtterance === u) webUtterance = null; finish(); };
    u.onerror = () => { if (webUtterance === u) webUtterance = null; finish(); };
    webUtterance = u;
    W.speechSynthesis.speak(u);
    return;
  }

  // Native (Expo Go bundles expo-speech, so this stays Expo-Go-compatible).
  Speech.stop();
  Speech.speak(text, {
    language: lang,
    rate,
    onDone: finish,
    onStopped: finish,
    onError: finish,
  });
}

/** Retained so Chrome can't GC the utterance mid-speech — a collected utterance
 *  hangs `speechSynthesis` and every later speak() (incl. the SpeakButton) goes
 *  silent until the engine is reset. */
let webUtterance: unknown = null;

let speechUnlocked = false;

/** iPhone Safari ignores speechSynthesis until one speak() runs inside a tap.
 *  Call from a tap handler; a silent utterance is enough. Web only, once. */
export function unlockSpeech(): void {
  if (Platform.OS !== 'web' || speechUnlocked) return;
  const W = globalThis as any;
  if (!W?.speechSynthesis || !W?.SpeechSynthesisUtterance) return;
  const u = new W.SpeechSynthesisUtterance(' ');
  u.volume = 0;
  webUtterance = u;
  W.speechSynthesis.speak(u);
  speechUnlocked = true;
}

/** Stop any in-flight speech on either platform. */
export function stopSpeaking(): void {
  if (Platform.OS === 'web') {
    const W = globalThis as any;
    W?.speechSynthesis?.cancel?.();
    return;
  }
  Speech.stop();
}
