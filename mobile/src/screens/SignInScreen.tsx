import { useState } from 'react';
import {
  ActivityIndicator,
  Image,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { ServerConnectionModal } from '../components/ServerConnectionModal';
import { hostOf } from '../data/live';
import { accountErrorText } from '../lib/errors';
import { useServer } from '../server/ServerContext';
import { colors, font, fontDisplay, radius, spacing } from '../theme';

/**
 * Shown instead of the app when the server keeps a deck per account
 * (required auth mode) and this device has no session. Open mode never
 * reaches it. "Create an account" appears only while the server's signup is
 * open; otherwise the learner is told who can let them in.
 */
export default function SignInScreen() {
  const server = useServer();
  const [creating, setCreating] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [connOpen, setConnOpen] = useState(false);
  const canCreate = server.signupOpen;
  const mode = creating && canCreate ? 'create' : 'signin';

  const submit = async () => {
    if (busy) return;
    const addr = email.trim();
    if (!addr.includes('@')) {
      setError('Enter the email address for your account.');
      return;
    }
    if (mode === 'create' && password.length < 8) {
      setError('Use a password of at least 8 characters.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      if (mode === 'create') await server.signup(addr, password);
      else await server.login(addr, password);
      setPassword('');
    } catch (e) {
      setError(accountErrorText(e, mode));
    } finally {
      setBusy(false);
    }
  };

  const switchMode = () => {
    setCreating(!creating);
    setError('');
  };

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={styles.root}>
      <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
        <View style={styles.card}>
          <Image
            source={require('../../assets/moon.png')}
            style={styles.logo}
            resizeMode="contain"
            accessibilityIgnoresInvertColors
          />
          <Text style={styles.title} accessibilityRole="header">
            {mode === 'create' ? 'Create your account' : 'Sign in to Yume'}
          </Text>
          <Text style={styles.copy}>
            {mode === 'create'
              ? 'Your account gets its own deck on this server. You’ll sign in with this email and password.'
              : 'This server keeps a separate deck for each person. Sign in to continue with your words and reviews.'}
          </Text>
          {server.authNotice && mode === 'signin' ? (
            <Text style={styles.notice}>{server.authNotice}</Text>
          ) : null}

          <Text style={styles.label}>Email</Text>
          <TextInput
            style={styles.input}
            accessibilityLabel="Email"
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="email-address"
            autoComplete="email"
            textContentType="username"
            editable={!busy}
          />
          <Text style={styles.label}>Password</Text>
          <TextInput
            style={styles.input}
            accessibilityLabel="Password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoComplete={mode === 'create' ? 'new-password' : 'current-password'}
            textContentType={mode === 'create' ? 'newPassword' : 'password'}
            onSubmitEditing={() => void submit()}
            editable={!busy}
          />
          {mode === 'create' ? <Text style={styles.hint}>At least 8 characters.</Text> : null}
          {error ? (
            <Text accessibilityRole="alert" style={styles.error}>
              {error}
            </Text>
          ) : null}

          <Pressable
            accessibilityRole="button"
            disabled={busy || !email.trim() || !password}
            style={({ pressed }) => [
              styles.button,
              (busy || !email.trim() || !password) && styles.buttonDisabled,
              pressed && styles.pressed,
            ]}
            onPress={() => void submit()}
          >
            {busy ? (
              <ActivityIndicator color={colors.onAccent} />
            ) : (
              <Text style={styles.buttonText}>{mode === 'create' ? 'Create account' : 'Sign in'}</Text>
            )}
          </Pressable>

          {canCreate ? (
            <Pressable accessibilityRole="button" disabled={busy} onPress={switchMode} hitSlop={8}>
              <Text style={styles.link}>
                {mode === 'create' ? 'Already have an account? Sign in' : 'New here? Create an account'}
              </Text>
            </Pressable>
          ) : (
            <Text style={styles.footnote}>New to this server? Ask the person who runs it to let you join.</Text>
          )}
          {mode === 'signin' ? (
            <Text style={styles.footnote}>Forgot your password? The person who runs this server can reset it.</Text>
          ) : null}

          <View style={styles.serverRow}>
            <Text style={styles.footnote}>Server: {hostOf(server.url) ?? 'none'}</Text>
            {/* The served web app belongs to its own server; only a native
                install chose its address and may need another. */}
            {Platform.OS !== 'web' ? (
              <Pressable accessibilityRole="button" onPress={() => setConnOpen(true)} hitSlop={8}>
                <Text style={styles.smallLink}>Use a different server</Text>
              </Pressable>
            ) : null}
          </View>
        </View>
      </ScrollView>
      <ServerConnectionModal visible={connOpen} onClose={() => setConnOpen(false)} />
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.surface0 },
  container: { flexGrow: 1, justifyContent: 'center', padding: spacing.lg },
  card: { width: '100%', maxWidth: 420, alignSelf: 'center', gap: spacing.sm },
  logo: { width: 48, height: 48, alignSelf: 'center', marginBottom: spacing.sm },
  title: {
    fontFamily: fontDisplay,
    fontSize: font.xl,
    color: colors.textPrimary,
    textAlign: 'center',
  },
  copy: {
    fontSize: font.sm,
    color: colors.textSecondary,
    lineHeight: font.sm + 8,
    textAlign: 'center',
    marginBottom: spacing.sm,
  },
  notice: {
    fontSize: font.sm,
    color: colors.textPrimary,
    backgroundColor: colors.accentMuted,
    borderRadius: radius.md,
    padding: spacing.sm,
    textAlign: 'center',
  },
  label: { fontSize: font.sm, color: colors.textPrimary, marginTop: spacing.xs },
  input: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 12,
    color: colors.textPrimary,
    fontSize: font.md,
  },
  hint: { fontSize: font.xs, color: colors.textTertiary },
  error: { fontSize: font.sm, color: colors.statusUnknown, lineHeight: font.sm + 6 },
  button: {
    backgroundColor: colors.accent,
    borderRadius: radius.pill,
    minHeight: 48,
    justifyContent: 'center',
    alignItems: 'center',
    marginTop: spacing.sm,
  },
  buttonDisabled: { opacity: 0.5 },
  pressed: { opacity: 0.8 },
  buttonText: { color: colors.onAccent, fontSize: font.md, fontWeight: '600' },
  link: {
    color: colors.accentText,
    fontSize: font.sm,
    fontWeight: '600',
    textAlign: 'center',
    paddingVertical: spacing.sm,
  },
  footnote: {
    fontSize: font.xs,
    color: colors.textTertiary,
    textAlign: 'center',
    lineHeight: font.xs + 6,
  },
  serverRow: { marginTop: spacing.lg, gap: spacing.xs, alignItems: 'center' },
  smallLink: { fontSize: font.xs, fontWeight: '600', color: colors.accentText },
});
