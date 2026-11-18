import { useState } from 'react';
import {
  KeyboardAvoidingView,
  Modal,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { useServer } from '../server/ServerContext';
import { DictionaryMissingNotice } from './DictionaryMissingNotice';
import { colors, font, radius, spacing } from '../theme';

/**
 * Server-URL editor, opened from Settings → Connection. Owner-only surface —
 * other accounts just use whatever server the app is pointed at, so they get
 * the status row without the plumbing.
 */
export function ServerConnectionModal({
  visible,
  onClose,
}: {
  visible: boolean;
  onClose: () => void;
}) {
  const server = useServer();
  const [draft, setDraft] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    setSaving(true);
    try {
      await server.setUrl(draft ?? server.url);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.backdrop}
      >
        <Pressable style={StyleSheet.absoluteFill} onPress={onClose} />
        <View style={styles.sheet}>
          <Text style={styles.sheetTitle}>Connect your server</Text>
          <Text style={styles.sheetHint}>
            Enter an address this phone can reach, such as your Raspberry Pi’s Tailscale address:{'\n'}
            http://yume-pi.your-tailnet.ts.net:7654
          </Text>
          <TextInput
            style={styles.input}
            value={draft ?? server.url}
            onChangeText={setDraft}
            placeholder="http://your-server:7654"
            placeholderTextColor={colors.textTertiary}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
          />
          {server.status === 'offline' && server.lastError ? (
            <Text style={styles.error} numberOfLines={2}>{server.lastError}</Text>
          ) : null}
          {server.status === 'online' && server.health ? (
            <Text style={styles.ok}>
              Connected, server {server.health.version}
              {server.health.dict?.loaded ? ` · ${server.health.dict.entries.toLocaleString()} dictionary entries` : ''}
            </Text>
          ) : null}
          <DictionaryMissingNotice />
          <View style={styles.sheetRow}>
            <Pressable
              style={({ pressed }) => [styles.btn, styles.btnGhost, pressed && styles.pressed]}
              onPress={() => { void server.setUrl(''); setDraft(''); }}
              accessibilityRole="button"
            >
              <Text style={styles.btnGhostLabel}>Try sample cards</Text>
            </Pressable>
            <Pressable
              style={({ pressed }) => [styles.btn, styles.btnPrimary, pressed && styles.pressed]}
              onPress={() => { void save(); }}
              disabled={saving}
              accessibilityRole="button"
            >
              <Text style={styles.btnPrimaryLabel}>{saving ? 'Connecting…' : 'Connect'}</Text>
            </Pressable>
          </View>
          <Pressable style={styles.closeRow} onPress={onClose} accessibilityRole="button">
            <Text style={styles.closeLabel}>Close</Text>
          </Pressable>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: 'center',
    padding: spacing.lg,
    backgroundColor: colors.backdrop,
  },
  sheet: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.lg,
    padding: spacing.lg,
    gap: spacing.sm,
  },
  sheetTitle: {
    fontSize: font.lg,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  sheetHint: {
    fontSize: font.xs,
    lineHeight: 16,
    color: colors.textSecondary,
  },
  input: {
    backgroundColor: colors.surface2,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 10,
    color: colors.textPrimary,
    fontSize: font.sm,
  },
  error: {
    fontSize: font.xs,
    color: colors.statusUnknown,
  },
  ok: {
    fontSize: font.xs,
    color: colors.statusKnown,
  },
  sheetRow: {
    flexDirection: 'row',
    gap: spacing.sm,
    marginTop: spacing.xs,
  },
  btn: {
    flex: 1,
    borderRadius: radius.md,
    paddingVertical: 11,
    alignItems: 'center',
  },
  btnPrimary: {
    backgroundColor: colors.accent,
  },
  btnPrimaryLabel: {
    color: colors.onAccent,
    fontSize: font.sm,
    fontWeight: '600',
  },
  btnGhost: {
    borderColor: colors.border,
    borderWidth: 1,
  },
  btnGhostLabel: {
    color: colors.textSecondary,
    fontSize: font.sm,
  },
  pressed: {
    opacity: 0.8,
  },
  closeRow: {
    alignItems: 'center',
    paddingTop: spacing.xs,
  },
  closeLabel: {
    fontSize: font.sm,
    color: colors.textTertiary,
  },
});
