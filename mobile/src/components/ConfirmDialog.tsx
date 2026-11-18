import { Modal, Pressable, StyleSheet, Text, View } from 'react-native';
import { colors, font, radius, spacing } from '../theme';

/**
 * Cross-platform confirmation dialog. We use this instead of React Native's
 * `Alert.alert`, which is a no-op on react-native-web (so the web build's
 * confirm prompts silently never appeared). A plain <Modal> renders the same
 * on web and native.
 */
export function ConfirmDialog({
  visible,
  title,
  message,
  confirmLabel = 'Delete',
  cancelLabel = 'Cancel',
  destructive = true,
  onConfirm,
  onCancel,
}: {
  visible: boolean;
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable style={styles.backdrop} onPress={onCancel}>
        <Pressable style={styles.sheet} onPress={(e) => e.stopPropagation()}>
          <Text style={styles.title}>{title}</Text>
          {message ? <Text style={styles.message}>{message}</Text> : null}
          <View style={styles.actions}>
            <Pressable
              style={({ pressed }) => [styles.btn, styles.cancel, pressed && styles.pressed]}
              onPress={onCancel}
              accessibilityRole="button"
            >
              <Text style={styles.cancelLabel}>{cancelLabel}</Text>
            </Pressable>
            <Pressable
              style={({ pressed }) => [
                styles.btn,
                destructive ? styles.confirmDestructive : styles.confirm,
                pressed && styles.pressed,
              ]}
              onPress={onConfirm}
              accessibilityRole="button"
            >
              <Text style={destructive ? styles.confirmDestructiveLabel : styles.confirmLabel}>
                {confirmLabel}
              </Text>
            </Pressable>
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: colors.backdrop,
    justifyContent: 'center',
    padding: spacing.lg,
  },
  sheet: {
    backgroundColor: colors.surface2,
    borderRadius: radius.lg,
    borderColor: colors.border,
    borderWidth: 1,
    padding: spacing.lg,
    gap: spacing.sm,
  },
  title: {
    fontSize: font.lg,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  message: {
    fontSize: font.sm,
    color: colors.textSecondary,
    lineHeight: font.sm + 6,
  },
  actions: {
    flexDirection: 'row',
    justifyContent: 'flex-end',
    gap: spacing.sm,
    marginTop: spacing.md,
  },
  btn: {
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.md,
  },
  pressed: {
    opacity: 0.7,
  },
  cancel: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
  },
  cancelLabel: {
    color: colors.textSecondary,
    fontSize: font.sm,
    fontWeight: '600',
  },
  confirm: {
    backgroundColor: colors.accentMuted,
  },
  confirmLabel: {
    color: colors.accentText,
    fontSize: font.sm,
    fontWeight: '600',
  },
  confirmDestructive: {
    backgroundColor: colors.statusUnknown,
  },
  confirmDestructiveLabel: {
    color: colors.surface0,
    fontSize: font.sm,
    fontWeight: '700',
  },
});
