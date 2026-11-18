import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

const KEY = 'yume.settings';

/** Furigana display policy over kanji words. */
export type FuriganaMode = 'off' | 'unknown' | 'all';

/** Which voice plays a review card: the stored generated clip (e.g. VOICEVOX)
 *  or the device's default text-to-speech voice. */
export type CardAudioMode = 'generated' | 'default';

/** English (word meaning + translation) on the answer side: always visible, or
 *  hidden behind a tap so you can recall it first. */
export type EnglishReveal = 'show' | 'tap';

/** Reader-controlled size for Japanese text (headwords, sentences, readings).
 *  One fixed size never fits everyone, and JP legibility is the whole point
 *  of the app. */
export type TextSize = 'standard' | 'large' | 'xlarge';

export const TEXT_SCALE: Record<TextSize, number> = {
  standard: 1,
  large: 1.15,
  xlarge: 1.3,
};

/** Where a word's meaning comes from: JMdict's English, or the Japanese
 *  definition from a Japanese-Japanese dictionary imported on the server. */
export type DefinitionLang = 'en' | 'ja';

export interface Settings {
  /** When to show furigana: never / only words you don't know / every kanji word. */
  furigana: FuriganaMode;
  /** Show Tanaka-corpus example sentences on the review card back. */
  showExamples: boolean;
  /** English (default) or Japanese definitions on cards and in lookups. */
  definitionLang: DefinitionLang;
  /** Also show the other language under the definition, in a quieter line. */
  secondDefinition: boolean;
  /** Start mnemonics folded on every card; tap the header to open one. */
  collapseMnemonics: boolean;
  /** Dot the mnemonic tiles whose part has not appeared in any Known word. */
  markNewParts: boolean;
  /** Voice for card pronunciation: stored generated clip vs device TTS. */
  cardAudio: CardAudioMode;
  /** Auto-play the card's pronunciation when the answer is revealed. */
  autoPlayAudio: boolean;
  /** Name the generated clip's voice under the audio row. Off keeps the
   *  credit in About only. */
  showVoiceCredit: boolean;
  /** Show English immediately, or hide it until tapped (active recall). */
  englishReveal: EnglishReveal;
  /** New words granted per 🌱 "Learn new words" press. Sessions never
   *  introduce new cards on their own — the button is the only entry, so
   *  reviews stay pure reviews and new words arrive only when asked for. */
  newPerLearn: number;
  /** A card rated Again comes back later in the SAME sitting until it earns
   *  a non-Again rating, instead of leaving the session and returning
   *  whenever FSRS schedules it (desktop behavior). */
  againRepeats: boolean;
  /** Multiplier bucket for Japanese text across the app. */
  textSize: TextSize;
}

const DEFAULTS: Settings = {
  furigana: 'unknown',
  showExamples: true,
  definitionLang: 'en',
  secondDefinition: false,
  collapseMnemonics: false,
  markNewParts: false,
  cardAudio: 'generated',
  autoPlayAudio: true,
  showVoiceCredit: true,
  englishReveal: 'show',
  newPerLearn: 10,
  againRepeats: true,
  textSize: 'standard',
};

interface SettingsState {
  settings: Settings;
  /** Whether persisted settings have loaded. */
  loaded: boolean;
  set: <K extends keyof Settings>(key: K, value: Settings[K]) => void;
  /** Overlay the server's per-account copy (server wins over device storage —
   *  the device copy is just a cache the browser may have purged). Persists
   *  the merged result locally. Called by <SettingsSync/>, not screens. */
  hydrate: (value: Partial<Settings>) => void;
}

const SettingsContext = createContext<SettingsState | null>(null);

/** Normalize deprecated keys from persisted device or server settings. */
function migrateStored(raw: Record<string, unknown>): Partial<Settings> {
  const {
    newCardsPerDay,
    reviewsFirst: _reviewsFirst,
    dailyTarget: _dailyTarget,
    ...rest
  } = raw as {
    newCardsPerDay?: number;
    reviewsFirst?: boolean;
    dailyTarget?: number;
  } & Partial<Settings>;
  if (rest.newPerLearn == null && typeof newCardsPerDay === 'number') {
    rest.newPerLearn = newCardsPerDay;
  }
  return rest;
}

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<Settings>(DEFAULTS);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    AsyncStorage.getItem(KEY)
      .then((raw) => {
        if (raw) {
          try {
            setSettings({ ...DEFAULTS, ...migrateStored(JSON.parse(raw)) });
          } catch {
            /* corrupt value — fall back to defaults */
          }
        }
      })
      .finally(() => setLoaded(true));
  }, []);

  const set = useCallback<SettingsState['set']>((key, value) => {
    setSettings((prev) => {
      const next = { ...prev, [key]: value };
      void AsyncStorage.setItem(KEY, JSON.stringify(next)).catch(() => {});
      return next;
    });
  }, []);

  const hydrate = useCallback<SettingsState['hydrate']>((value) => {
    setSettings((prev) => {
      const next = { ...prev, ...migrateStored(value) };
      void AsyncStorage.setItem(KEY, JSON.stringify(next)).catch(() => {});
      return next;
    });
  }, []);

  const value = useMemo(
    () => ({ settings, loaded, set, hydrate }),
    [settings, loaded, set, hydrate],
  );
  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

export function useSettings(): SettingsState {
  const ctx = useContext(SettingsContext);
  if (!ctx) throw new Error('useSettings must be used inside <SettingsProvider>');
  return ctx;
}

/** The active Japanese-text multiplier. Apply to JP headwords, sentences, and
 *  readings only — UI chrome stays on the fixed `font` scale. */
export function useTextScale(): number {
  return TEXT_SCALE[useSettings().settings.textSize];
}
