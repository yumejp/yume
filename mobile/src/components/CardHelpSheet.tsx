import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Linking,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import type { MnemonicPart } from '../api/types';
import type { CardEdit } from '../lib/cardEdits';
import { colors, font, radius, spacing } from '../theme';
import { Icon, type IconName } from './Icon';

export interface CardHelpAction {
  tag: string;
  icon: IconName;
  title: string;
  detail: string;
}

export const CARD_HELP_ACTIONS: CardHelpAction[] = [
  { tag: 'needs-explanation', icon: 'robot-happy-outline', title: 'Explain this word', detail: 'Add a short note about meaning, nuance, or memory clues.' },
  { tag: 'needs-sentence', icon: 'cards-outline', title: 'Add an example sentence', detail: 'Add a natural Japanese sentence with an English translation.' },
  { tag: 'needs-audio', icon: 'volume-high', title: 'Add or fix audio', detail: 'Create clear pronunciation audio for the word and sentence.' },
  { tag: 'needs-fix', icon: 'pencil-outline', title: 'Correct card details', detail: 'Check the word, reading, definition, and sentence for mistakes.' },
];

/** How a tutor is connected, and what it does with a request. */
export const TUTOR_GUIDE_URL = 'https://github.com/yumejp/yume/blob/main/docs/ai-cards.md';

/** What the learner can already see on the card, to start their edits from. */
export interface CardHelpContent {
  sentence: string;
  translation: string;
  explanation: string;
  mnemonicParts: MnemonicPart[];
  mnemonicStory: string;
  /** The mnemonic shown is the server's shared one, not the card's own. */
  mnemonicShared: boolean;
}

type EditorKind = CardEdit['kind'];

export function CardHelpSheet({
  visible,
  lemma,
  isRequested,
  savingTag,
  error,
  onToggle,
  onClose,
  content,
  onSaveEdit,
  editSaving,
  editError,
  onEditorChange,
}: {
  visible: boolean;
  lemma: string;
  isRequested: (tag: string) => boolean;
  savingTag: string | null;
  error: string | null;
  onToggle: (tag: string) => void;
  onClose: () => void;
  content: CardHelpContent;
  /** Resolves true once the server has the edit. */
  onSaveEdit: (edit: CardEdit) => Promise<boolean>;
  editSaving: boolean;
  editError: string | null;
  /** Opening or leaving an editor; clears a stale save error. */
  onEditorChange?: () => void;
}) {
  const [editor, setEditor] = useState<EditorKind | null>(null);
  const [story, setStory] = useState('');
  const [sentence, setSentence] = useState('');
  const [sentenceEnglish, setSentenceEnglish] = useState('');
  const [translation, setTranslation] = useState('');
  const [explanation, setExplanation] = useState('');
  const [saved, setSaved] = useState<string | null>(null);

  // Every opening starts at the list, for whichever card is showing.
  useEffect(() => {
    if (!visible) {
      setEditor(null);
      setSaved(null);
    }
  }, [visible]);

  const open = (kind: EditorKind) => {
    onEditorChange?.();
    setSaved(null);
    setStory(content.mnemonicStory);
    setSentence('');
    setSentenceEnglish('');
    setTranslation(content.translation);
    setExplanation(content.explanation);
    setEditor(kind);
  };
  const back = () => {
    onEditorChange?.();
    setEditor(null);
  };

  const hasSentence = content.sentence !== '';
  const edit: CardEdit | null =
    editor === 'mnemonic'
      ? story.trim() && story.trim() !== content.mnemonicStory ? { kind: 'mnemonic', story } : null
      : editor === 'example'
        ? sentence.trim() ? { kind: 'example', sentence, translation: sentenceEnglish } : null
        : editor === 'meaning'
          ? translation.trim() !== content.translation || explanation.trim() !== content.explanation
            ? { kind: 'meaning', explanation, ...(hasSentence ? { translation } : {}) }
            : null
          : null;

  const save = async () => {
    if (!edit || editSaving) return;
    if (await onSaveEdit(edit)) {
      setEditor(null);
      setSaved(
        edit.kind !== 'example'
          ? 'Saved to this card.'
          : hasSentence
            ? 'Example added. It shows under Examples on this card.'
            : 'Example added. It shows as this card’s sentence.',
      );
    }
  };

  const writeActions: { kind: EditorKind; icon: IconName; title: string; detail: string; state: string }[] = [
    {
      kind: 'mnemonic',
      icon: 'star-four-points-outline',
      title: 'Write my own mnemonic',
      detail: 'A short story that links the word to its meaning.',
      state: content.mnemonicStory ? 'Edit' : 'Write',
    },
    {
      kind: 'example',
      icon: 'plus',
      title: 'Add my own example',
      detail: 'A Japanese sentence you want to remember, with English if you like.',
      state: 'Add',
    },
    {
      kind: 'meaning',
      icon: 'playlist-edit',
      title: hasSentence ? 'Fix the translation or explanation' : 'Write an explanation',
      detail: hasSentence
        ? 'Correct the sentence’s English, or note nuance and usage.'
        : 'Note nuance, register, or a mistake to avoid.',
      state: 'Edit',
    },
  ];

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={editor ? back : onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={styles.backdrop}>
        {/* A stray tap outside must not throw away a draft. */}
        <Pressable style={StyleSheet.absoluteFill} onPress={editor ? undefined : onClose} />
        <View style={styles.sheet}>
          <ScrollView contentContainerStyle={styles.sheetBody} keyboardShouldPersistTaps="handled">
            {editor ? (
              <>
                <Text style={styles.eyebrow}>WRITE IT YOURSELF</Text>
                <Text style={styles.title}>{lemma}</Text>
                {editor === 'mnemonic' ? (
                  <>
                    {content.mnemonicParts.length > 0 ? (
                      <Text style={styles.context}>
                        {content.mnemonicParts.map((p) => `${p.kanji} ${p.meaning}`).join(' · ')}
                      </Text>
                    ) : null}
                    <Text style={styles.label}>Your mnemonic</Text>
                    <TextInput
                      style={[styles.input, styles.inputTall]}
                      value={story}
                      onChangeText={setStory}
                      placeholder="One to three sentences, ending with a short example"
                      placeholderTextColor={colors.textTertiary}
                      maxLength={400}
                      multiline
                      accessibilityLabel="Your mnemonic"
                    />
                    <Text style={styles.hint}>
                      {content.mnemonicShared
                        ? 'Your version replaces the shared mnemonic on this card only.'
                        : content.mnemonicParts.length > 0
                          ? 'Your story replaces this card’s story. The kanji parts stay.'
                          : 'Your story shows under Mnemonic on the back of the card.'}
                    </Text>
                  </>
                ) : editor === 'example' ? (
                  <>
                    <Text style={styles.label}>Japanese sentence</Text>
                    <TextInput
                      style={[styles.input, styles.inputJp]}
                      value={sentence}
                      onChangeText={setSentence}
                      placeholder={`A sentence that uses ${lemma}`}
                      placeholderTextColor={colors.textTertiary}
                      multiline
                      accessibilityLabel="Japanese sentence"
                    />
                    <Text style={styles.label}>English (optional)</Text>
                    <TextInput
                      style={styles.input}
                      value={sentenceEnglish}
                      onChangeText={setSentenceEnglish}
                      placeholder="What the sentence means"
                      placeholderTextColor={colors.textTertiary}
                      multiline
                      accessibilityLabel="English of the sentence"
                    />
                  </>
                ) : (
                  <>
                    {hasSentence ? (
                      <>
                        <Text style={styles.context}>{content.sentence}</Text>
                        <Text style={styles.label}>English of the sentence</Text>
                        <TextInput
                          style={styles.input}
                          value={translation}
                          onChangeText={setTranslation}
                          placeholder="What the sentence means"
                          placeholderTextColor={colors.textTertiary}
                          multiline
                          accessibilityLabel="English of the sentence"
                        />
                      </>
                    ) : null}
                    <Text style={styles.label}>Explanation</Text>
                    <TextInput
                      style={[styles.input, styles.inputTall]}
                      value={explanation}
                      onChangeText={setExplanation}
                      placeholder="Nuance, register, or a mistake to avoid"
                      placeholderTextColor={colors.textTertiary}
                      multiline
                      accessibilityLabel="Explanation"
                    />
                    <Text style={styles.hint}>Leave a field empty to remove it from the card.</Text>
                  </>
                )}
                {editError ? <Text style={styles.error}>{editError}</Text> : null}
                <View style={styles.buttonRow}>
                  <Pressable
                    style={({ pressed }) => [styles.btn, styles.btnGhost, pressed && styles.pressed]}
                    onPress={back}
                    disabled={editSaving}
                    accessibilityRole="button"
                  >
                    <Text style={styles.btnGhostLabel}>Cancel</Text>
                  </Pressable>
                  <Pressable
                    style={({ pressed }) => [styles.btn, styles.btnPrimary, !edit && styles.btnDisabled, pressed && styles.pressed]}
                    onPress={() => { void save(); }}
                    disabled={!edit || editSaving}
                    accessibilityRole="button"
                    accessibilityState={{ disabled: !edit || editSaving, busy: editSaving }}
                  >
                    {editSaving ? <ActivityIndicator size="small" color={colors.onAccent} /> : (
                      <Text style={styles.btnPrimaryLabel}>Save</Text>
                    )}
                  </Pressable>
                </View>
              </>
            ) : (
              <>
                <Text style={styles.eyebrow}>HELP WITH THIS CARD</Text>
                <Text style={styles.title}>{lemma}</Text>
                {saved ? <Text style={styles.saved} accessibilityLiveRegion="polite">{saved}</Text> : null}
                <Text style={styles.section}>Ask your tutor</Text>
                <Text style={styles.intro}>
                  A request waits on this card until you ask your AI tutor to check your flagged cards. Yume does not contact a tutor by itself. No tutor? Write it yourself below.
                </Text>
                <Pressable
                  onPress={() => { void Linking.openURL(TUTOR_GUIDE_URL); }}
                  accessibilityRole="link"
                  hitSlop={8}
                  style={styles.guideLink}
                >
                  <Text style={styles.link}>How to connect a tutor</Text>
                </Pressable>
                <View style={styles.actions}>
                  {CARD_HELP_ACTIONS.map((action) => {
                    const active = isRequested(action.tag);
                    const saving = savingTag === action.tag;
                    return (
                      <Pressable
                        key={action.tag}
                        style={[styles.action, active && styles.actionActive]}
                        onPress={() => onToggle(action.tag)}
                        disabled={savingTag != null}
                        accessibilityRole="button"
                        accessibilityState={{ selected: active, busy: saving }}
                      >
                        <Icon name={action.icon} size={20} color={active ? colors.accentText : colors.textSecondary} />
                        <View style={styles.actionText}>
                          <Text style={styles.actionTitle}>{action.title}</Text>
                          <Text style={styles.actionDetail}>{action.detail}</Text>
                        </View>
                        {saving ? <ActivityIndicator size="small" color={colors.accentText} /> : (
                          <Text style={[styles.state, active && styles.stateActive]}>{active ? 'Requested' : 'Request'}</Text>
                        )}
                      </Pressable>
                    );
                  })}
                </View>
                {error ? <Text style={styles.error}>{error}</Text> : null}
                <Text style={styles.privacy}>A request only adds a tag to the card on your server. Tap Requested again to remove it.</Text>
                <Text style={styles.section}>Write it yourself</Text>
                <View style={styles.actions}>
                  {writeActions.map((action) => (
                    <Pressable
                      key={action.kind}
                      style={styles.action}
                      onPress={() => open(action.kind)}
                      accessibilityRole="button"
                    >
                      <Icon name={action.icon} size={20} color={colors.textSecondary} />
                      <View style={styles.actionText}>
                        <Text style={styles.actionTitle}>{action.title}</Text>
                        <Text style={styles.actionDetail}>{action.detail}</Text>
                      </View>
                      <Text style={styles.state}>{action.state}</Text>
                    </Pressable>
                  ))}
                </View>
                <Pressable style={styles.close} onPress={onClose} accessibilityRole="button">
                  <Text style={styles.closeLabel}>Done</Text>
                </Pressable>
              </>
            )}
          </ScrollView>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: colors.backdrop, justifyContent: 'flex-end' },
  sheet: { maxHeight: '92%', backgroundColor: colors.surface1, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, borderColor: colors.border, borderWidth: 1 },
  sheetBody: { padding: spacing.lg, paddingBottom: spacing.xl, gap: spacing.sm },
  eyebrow: { fontSize: font.sm, color: colors.textTertiary },
  title: { fontSize: font.xl, color: colors.textPrimary, fontWeight: '700' },
  section: { fontSize: font.md, color: colors.textPrimary, fontWeight: '700', marginTop: spacing.sm },
  intro: { fontSize: font.sm, color: colors.textSecondary, lineHeight: font.sm + 6 },
  guideLink: { alignSelf: 'flex-start', marginBottom: spacing.xs },
  link: { fontSize: font.sm, color: colors.accentText, fontWeight: '600', textDecorationLine: 'underline' },
  actions: { gap: spacing.sm },
  action: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, borderColor: colors.border, borderWidth: 1, borderRadius: radius.md, padding: spacing.md },
  actionActive: { borderColor: colors.accent, backgroundColor: colors.accentMuted },
  actionText: { flex: 1, gap: 2 },
  actionTitle: { fontSize: font.md, color: colors.textPrimary, fontWeight: '600' },
  actionDetail: { fontSize: font.xs, color: colors.textTertiary, lineHeight: font.xs + 5 },
  state: { fontSize: font.xs, color: colors.textSecondary, fontWeight: '600' },
  stateActive: { color: colors.accentText },
  error: { fontSize: font.sm, color: colors.statusUnknown },
  saved: { fontSize: font.sm, color: colors.statusKnown },
  privacy: { fontSize: font.xs, color: colors.textTertiary, lineHeight: font.xs + 5 },
  context: { fontSize: font.md, color: colors.textSecondary, lineHeight: font.md + 8 },
  label: { fontSize: font.sm, color: colors.textSecondary, fontWeight: '600', marginTop: spacing.xs },
  hint: { fontSize: font.xs, color: colors.textTertiary, lineHeight: font.xs + 5 },
  input: {
    backgroundColor: colors.surface2,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: font.sm,
    minHeight: 44,
    textAlignVertical: 'top',
  },
  inputJp: { fontSize: font.jp, minHeight: 64 },
  inputTall: { minHeight: 96 },
  buttonRow: { flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm },
  btn: { flex: 1, minHeight: 44, borderRadius: radius.md, alignItems: 'center', justifyContent: 'center' },
  btnPrimary: { backgroundColor: colors.accent },
  btnDisabled: { opacity: 0.5 },
  btnPrimaryLabel: { color: colors.onAccent, fontSize: font.sm, fontWeight: '600' },
  btnGhost: { borderColor: colors.border, borderWidth: 1 },
  btnGhostLabel: { color: colors.textSecondary, fontSize: font.sm },
  pressed: { opacity: 0.8 },
  close: { minHeight: 44, alignItems: 'center', justifyContent: 'center', marginTop: spacing.xs },
  closeLabel: { fontSize: font.md, color: colors.accentText, fontWeight: '700' },
});
