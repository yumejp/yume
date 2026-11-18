import { Platform } from 'react-native';
import { unlockSpeech } from './speech';

/**
 * Plays a card's generated-audio sidecar (e.g. a VOICEVOX .wav) from base64.
 *
 * Web: ONE reused HTMLAudioElement fed Blob URLs. iPhone Safari lets an
 * element play outside a tap only once it has played inside one, and a play()
 * after an awaited fetch counts as outside. So the element is unlocked in the
 * first tap (unlockAudio), clips are prefetched, and callers with cached bytes
 * call playCardAudio synchronously from the tap. A fresh `new Audio()` per clip
 * would start locked every time.
 * Native: expo-audio. AVPlayer/ExoPlayer don't reliably decode data: URIs, so
 * we stage the bytes to a cache file (expo-file-system legacy writeAsStringAsync
 * handles base64 directly) and play the file:// URI. setAudioModeAsync lets it
 * sound even with the phone's ring switch on silent — important for review on
 * the go.
 *
 * Both modules are bundled in Expo Go, so the app stays Expo-Go-runnable.
 * One clip plays at a time: a new call stops the previous player.
 */

let webAudio: HTMLAudioElement | null = null;
/** The Blob URL the web element is playing; revoked when the next replaces it. */
let webUrl: string | null = null;
let webUnlocked = false;
let nativePlayer: { play: () => void; remove: () => void } | null = null;
/** Bumped each native call so a replay never overwrites a file the previous
 *  (possibly still-playing) player holds open. */
let nativeSeq = 0;
/** The staged temp file the current native player is playing from. */
let nativePath: string | null = null;

// 20 ms of 8 kHz silence: played inside a tap to unlock the web element.
const SILENT_WAV = `data:audio/wav;base64,UklGRsQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YaAAAACA${'gICA'.repeat(53)}`;

function webElement(): HTMLAudioElement | null {
  if (!webAudio && typeof Audio !== 'undefined') webAudio = new Audio();
  return webAudio;
}

/**
 * Call from a tap handler (Start review, Reveal) before any await: unlocks the
 * shared audio element and the speech engine for playback outside taps, such
 * as auto-play on reveal. A no-op on native and once unlocked.
 */
export function unlockAudio(): void {
  if (Platform.OS !== 'web') return;
  unlockSpeech();
  const el = webElement();
  if (webUnlocked || !el || !el.paused) return;
  el.src = SILENT_WAV;
  el.play()
    .then(() => { webUnlocked = true; })
    .catch(() => { /* not a tap after all; the next one tries again */ });
}

/** True when the browser refused to play without a tap (NotAllowedError). */
export function isPlaybackBlocked(err: unknown): boolean {
  return (err as { name?: string } | null)?.name === 'NotAllowedError';
}

function base64Blob(base64: string, mime: string): Blob {
  const bin = atob(base64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return new Blob([bytes], { type: mime });
}

/** Stop the current clip and remove its staged native file. */
export async function stopCardAudio(): Promise<void> {
  if (Platform.OS === 'web') {
    try {
      webAudio?.pause();
    } catch {
      /* ignore */
    }
    return;
  }
  try {
    nativePlayer?.remove();
  } catch {
    /* ignore */
  }
  nativePlayer = null;
  if (nativePath) {
    const stale = nativePath;
    nativePath = null;
    try {
      const FileSystem = await import('expo-file-system/legacy');
      await FileSystem.deleteAsync(stale, { idempotent: true });
    } catch {
      /* best-effort — OS purges cache eventually */
    }
  }
}

function extFor(mime: string): string {
  if (mime.includes('wav')) return 'wav';
  if (mime.includes('mpeg') || mime.includes('mp3')) return 'mp3';
  if (mime.includes('webm')) return 'webm';
  if (mime.includes('ogg')) return 'ogg';
  if (mime.includes('mp4') || mime.includes('aac')) return 'm4a';
  return 'audio';
}

/** Play a clip. On the web, play() is called before this function's first
 *  await, so a tap handler that calls it directly keeps its gesture. Rejects
 *  when the browser blocks playback (see isPlaybackBlocked). */
export async function playCardAudio(mime: string, base64: string): Promise<void> {
  if (Platform.OS === 'web') {
    const el = webElement();
    if (!el) return;
    try {
      el.pause();
    } catch {
      /* ignore */
    }
    const url = URL.createObjectURL(base64Blob(base64, mime));
    if (webUrl) URL.revokeObjectURL(webUrl);
    webUrl = url;
    el.src = url;
    await el.play();
    webUnlocked = true;
    return;
  }

  // Native (iOS/Android): stage a temp file, then play it. Each call takes a
  // sequence number up front and re-checks it after every await — two rapid
  // taps interleave across the awaits, and without the check the earlier call
  // assigned its player/file AFTER the later one, so both clips played over
  // each other and the earlier player + temp file leaked.
  const seq = ++nativeSeq;
  const { createAudioPlayer, setAudioModeAsync } = await import('expo-audio');
  const FileSystem = await import('expo-file-system/legacy');
  try {
    await setAudioModeAsync({ playsInSilentMode: true });
  } catch {
    /* mode is best-effort */
  }
  // Stop the previous clip (and delete its temp file) BEFORE writing, and use
  // a fresh file each call, so an in-flight player never holds the path we're
  // about to overwrite (which made a replay tap silently fail).
  await stopCardAudio();
  if (seq !== nativeSeq) return; // superseded by a newer tap
  const path = `${FileSystem.cacheDirectory}yume-card-audio-${seq}.${extFor(mime)}`;
  await FileSystem.writeAsStringAsync(path, base64, {
    encoding: FileSystem.EncodingType.Base64,
  });
  if (seq !== nativeSeq) {
    // A newer tap won the race while we were writing — clean up our staging
    // instead of overwriting its player/path.
    void FileSystem.deleteAsync(path, { idempotent: true }).catch(() => {});
    return;
  }
  nativePlayer = createAudioPlayer({ uri: path });
  nativePath = path;
  nativePlayer.play();
}
