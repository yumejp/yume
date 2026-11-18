import { useState } from 'react';
import { StatusBar } from 'expo-status-bar';
import { Image, StyleSheet, View } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { TabBar, type TabKey } from './src/components/TabBar';
import DictionaryScreen from './src/screens/DictionaryScreen';
import LibraryScreen from './src/screens/LibraryScreen';
import ReviewScreen from './src/screens/ReviewScreen';
import SettingsScreen from './src/screens/SettingsScreen';
import SignInScreen from './src/screens/SignInScreen';
import { OfflineSync } from './src/server/OfflineSync';
import { ServerProvider, useServer } from './src/server/ServerContext';
import { SettingsProvider } from './src/settings/SettingsContext';
import { SettingsSync } from './src/settings/SettingsSync';
import { colors } from './src/theme';

function AppShell() {
  const [tab, setTab] = useState<TabKey>('review');
  const { loaded, needsSignIn } = useServer();

  // Wait for persisted connection state before choosing a data source.
  if (!loaded) {
    return (
      <View style={styles.splash}>
        <Image
          source={require('./assets/moon.png')}
          style={styles.splashLogo}
          resizeMode="contain"
          accessibilityLabel="Yume"
        />
      </View>
    );
  }

  // Required auth mode with no session: the tabs would only show 401s.
  if (needsSignIn) return <SignInScreen />;

  return (
    <>
      <View style={styles.body}>
        {/* Screens stay mounted (toggled via display) so per-tab state —
            an in-progress Review session, a Dictionary search — survives
            switching tabs instead of resetting on every visit. */}
        <View style={[styles.pane, tab === 'review' ? styles.shown : styles.hidden]}>
          <ReviewScreen active={tab === 'review'} onFindWord={() => setTab('dictionary')} />
        </View>
        <View style={[styles.pane, tab === 'library' ? styles.shown : styles.hidden]}>
          <LibraryScreen />
        </View>
        <View style={[styles.pane, tab === 'dictionary' ? styles.shown : styles.hidden]}>
          <DictionaryScreen onReview={() => setTab('review')} />
        </View>
        <View style={[styles.pane, tab === 'settings' ? styles.shown : styles.hidden]}>
          <SettingsScreen />
        </View>
      </View>
      <TabBar active={tab} onChange={setTab} />
    </>
  );
}

export default function App() {
  return (
    <GestureHandlerRootView style={styles.root}>
    <SafeAreaProvider>
      <SettingsProvider>
        <ServerProvider>
          <SettingsSync />
          <OfflineSync />
          <SafeAreaView style={styles.root} edges={['top', 'left', 'right']}>
            <StatusBar style={colors.scheme === 'light' ? 'dark' : 'light'} />
            <AppShell />
          </SafeAreaView>
        </ServerProvider>
      </SettingsProvider>
    </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: colors.surface0,
  },
  body: {
    flex: 1,
  },
  pane: {
    flex: 1,
  },
  shown: {
    display: 'flex',
  },
  hidden: {
    display: 'none',
  },
  splash: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  splashLogo: {
    width: 64,
    height: 64,
  },
});
