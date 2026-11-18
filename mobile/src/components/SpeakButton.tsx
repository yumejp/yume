import { Icon } from './Icon';
import { useEffect, useRef, useState } from 'react';
import { Pressable, StyleSheet, Text } from 'react-native';
import { speak, stopSpeaking } from '../lib/speech';
import { colors, font, radius, spacing } from '../theme';

interface Props {
  text: string;
  /** BCP-47 lang tag for the voice pick. Defaults to Japanese. */
  lang?: string;
  /** Override the label. Defaults to "Listen". */
  label?: string;
  /** Compact icon-only variant — fits next to dense content (examples, popup). */
  compact?: boolean;
  /** Slower rate for harder sentences. Default 0.95 = mildly slower than native. */
  rate?: number;
}

/**
 * TTS button over lib/speech's cross-platform speak():
 * - Web (Safari/Chrome): the free Web Speech API.
 * - Native (Expo Go / device): expo-speech.
 * The speaking indicator resets via speak()'s onDone, which fires on finish,
 * error, stop, and no-TTS alike.
 */
export function SpeakButton({
  text,
  lang = 'ja-JP',
  label = 'Listen',
  compact = false,
  rate = 0.95,
}: Props) {
  const [speaking, setSpeaking] = useState(false);
  const speakingRef = useRef(false);

  const setState = (on: boolean) => {
    speakingRef.current = on;
    setSpeaking(on);
  };

  // Stop only speech this button started — unmounting one Listen button must
  // not silence another's in-flight sentence.
  useEffect(() => {
    return () => {
      if (speakingRef.current) stopSpeaking();
    };
  }, []);

  if (!text) return null;

  const onPress = () => {
    if (speakingRef.current) {
      stopSpeaking(); // speak()'s onDone fires and resets state
      return;
    }
    setState(true);
    speak(text, { lang, rate, onDone: () => setState(false) });
  };

  return (
    <Pressable
      style={({ pressed }) => [
        compact ? styles.iconBtn : styles.btn,
        speaking && styles.btnActive,
        pressed && styles.btnPressed,
      ]}
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={speaking ? 'Stop reading' : `Read aloud: ${text.slice(0, 40)}`}
    >
      <Icon
        name={speaking ? 'stop' : 'volume-high'}
        size={compact ? 13 : 14}
        color={speaking ? colors.accentText : colors.textSecondary}
      />
      {compact ? null : (
        <Text style={[styles.label, speaking && styles.labelActive]}>
          {speaking ? 'Stop' : label}
        </Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  btn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingVertical: 6,
    paddingHorizontal: spacing.md,
    borderRadius: radius.sm,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface2,
  },
  iconBtn: {
    width: 28,
    height: 24,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: radius.sm,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface2,
  },
  btnActive: {
    borderColor: colors.accentText,
    backgroundColor: colors.accentMuted,
  },
  btnPressed: {
    opacity: 0.7,
  },
  label: {
    fontSize: font.sm,
    color: colors.textSecondary,
    fontWeight: '600',
  },
  labelActive: {
    color: colors.accentText,
  },
});
