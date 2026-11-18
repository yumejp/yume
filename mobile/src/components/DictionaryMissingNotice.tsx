import { Platform, StyleSheet, Text, View, type StyleProp, type ViewStyle } from 'react-native';
import type { HealthResponse } from '../api/types';
import { useServer } from '../server/ServerContext';
import { colors, font, radius, spacing } from '../theme';

type DictState = NonNullable<HealthResponse['dict']>;

/** The connected server's dictionary problem, or null when lookups work (or
 *  the server is offline, where the offline copy already explains). */
export function missingDictionary(status: string, health: HealthResponse | null): DictState | null {
  if (status !== 'online' || !health?.dict || health.dict.loaded) return null;
  return health.dict;
}

// Servers from before this notice said "JMdict_e.gz not found. …" instead.
const NOT_INSTALLED = /^dictionary not installed\b|^JMdict_e\.gz not found\b/;

/** True when the server has no dictionary file, as opposed to a damaged one. */
export function dictionaryNotInstalled(error: string): boolean {
  return NOT_INSTALLED.test(error);
}

/** The command inside the server's "dictionary not installed: run `…`" text. */
export function setupCommand(error: string): string | null {
  return dictionaryNotInstalled(error) ? (/`([^`]+)`/.exec(error)?.[1] ?? null) : null;
}

/**
 * Persistent notice while the server has no working dictionary. Search and
 * new words depend on it, and only whoever runs the server can fix it, so
 * the owner gets the exact command and everyone else who to ask.
 */
export function DictionaryMissingNotice({ style }: { style?: StyleProp<ViewStyle> }) {
  const server = useServer();
  const dict = missingDictionary(server.status, server.health);
  if (!dict) return null;
  const command = setupCommand(dict.error);
  const notInstalled = dictionaryNotInstalled(dict.error);
  return (
    <View style={[styles.box, style]} accessibilityRole="alert">
      <Text style={styles.title}>{notInstalled ? 'Dictionary not installed' : 'Dictionary failed to load'}</Text>
      {!server.isOwner ? (
        <Text style={styles.body}>
          Dictionary search needs it. Ask the person who runs your Yume server to install it.
        </Text>
      ) : command ? (
        <>
          <Text style={styles.body}>
            Dictionary search needs it. On the computer running Yume, run this command, then restart the server:
          </Text>
          <Text style={styles.code} selectable>{command}</Text>
        </>
      ) : (
        <>
          <Text style={styles.body}>
            {notInstalled
              ? 'Dictionary search needs it. On the computer running Yume, run yume-server setup, then restart the server.'
              : 'Dictionary search needs it. On the computer running Yume, delete JMdict_e.gz from the data folder, run yume-server setup, then restart the server. The server reported:'}
          </Text>
          {dict.error && !notInstalled ? <Text style={styles.code} selectable>{dict.error}</Text> : null}
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.xs,
  },
  title: {
    fontSize: font.sm,
    fontWeight: '700',
    color: colors.textPrimary,
  },
  body: {
    fontSize: font.xs,
    lineHeight: 17,
    color: colors.textSecondary,
  },
  code: {
    marginTop: spacing.xs,
    backgroundColor: colors.surface2,
    borderRadius: radius.sm,
    paddingHorizontal: spacing.sm,
    paddingVertical: 6,
    fontSize: font.xs,
    color: colors.textPrimary,
    fontFamily: Platform.select({ ios: 'Menlo', default: 'monospace' }),
  },
});
