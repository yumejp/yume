import { useCallback, useEffect, useState } from 'react';
import { Image, Linking, Modal, Platform, ScrollView, StyleSheet, Switch, Text, View, Pressable } from 'react-native';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ScreenHeader } from '../components/ScreenHeader';
import { DictionaryMissingNotice } from '../components/DictionaryMissingNotice';
import { ServerConnectionModal } from '../components/ServerConnectionModal';
import ProgressScreen from './ProgressScreen';
import { hostOf } from '../data/live';
import { useVacation } from '../data/vacation';
import { useReviewLimit } from '../data/reviewLimit';
import { useServer } from '../server/ServerContext';
import {
  useSettings,
  type CardAudioMode,
  type DefinitionLang,
  type FuriganaMode,
  type TextSize,
} from '../settings/SettingsContext';
import { colors, font, fontDisplay, radius, spacing } from '../theme';
import { voiceCreditSummary } from '../lib/voiceCredit';
import { CREDIT_LINKS, licensesUrl } from '../lib/credits';
import { errorText } from '../lib/errors';
import { deckFileName, formatBytes, saveBlobOnWeb, snapshotLabel, snapshotToPrune } from '../lib/yourData';
import type { BackupInfo, MediaVoice } from '../api/types';

const FURIGANA_OPTIONS: { value: FuriganaMode; label: string }[] = [
  { value: 'off', label: 'Off' },
  // One word: a two-word label wraps inside its third-width segment.
  { value: 'unknown', label: 'Unknown' },
  { value: 'all', label: 'All kanji' },
];

const TEXT_SIZE_OPTIONS: { value: TextSize; label: string }[] = [
  { value: 'standard', label: 'Standard' },
  { value: 'large', label: 'Large' },
  { value: 'xlarge', label: 'XL' },
];

const DEFINITION_OPTIONS: { value: DefinitionLang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ja', label: 'Japanese' },
];

const CARD_AUDIO_OPTIONS: { value: CardAudioMode; label: string }[] = [
  { value: 'generated', label: 'Saved audio' },
  { value: 'default', label: 'Device voice' },
];

const REVIEW_LIMIT_OPTIONS: { value: number; label: string }[] = [0, 100, 150, 200, 300].map((n) => ({
  value: n,
  label: n === 0 ? 'Off' : String(n),
}));

const NEW_PER_LEARN_OPTIONS: { value: number; label: string }[] = [3, 5, 10, 15].map((n) => ({
  value: n,
  label: String(n),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const APP_VERSION: string = require('../../package.json').version;

export default function SettingsScreen() {
  const { settings, set } = useSettings();
  const server = useServer();
  const vacation = useVacation();
  const reviewLimit = useReviewLimit();
  const [connOpen, setConnOpen] = useState(false);
  const [progressOpen, setProgressOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  // About names every credited voice in the deck, so the credit is complete
  // even when cards hide it.
  const [voices, setVoices] = useState<MediaVoice[]>([]);
  const [dictNames, setDictNames] = useState<string[]>([]);
  useEffect(() => {
    if (!aboutOpen || !server.client) return;
    let alive = true;
    server.client
      .listMediaVoices()
      .then((v) => alive && setVoices(v))
      .catch(() => {});
    server.client
      .listDicts()
      .then((d) => alive && setDictNames(d.filter((x) => x.enabled).map((x) => x.name)))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [aboutOpen, server.client]);

  return (
    <View style={styles.root}>
      <ScreenHeader title="Settings" />
      <ScrollView contentContainerStyle={styles.body}>

        <Section title="Cards">
          <Row label="Japanese text size" hint="Scales all Japanese text across the app" stacked>
            <Choice
              label="Japanese text size"
              options={TEXT_SIZE_OPTIONS}
              value={settings.textSize}
              onChange={(v) => set('textSize', v)}
              stretch
            />
          </Row>
          <Row label="Definitions" hint="Preferred definition language">
            <Choice
              label="Definitions"
              options={DEFINITION_OPTIONS}
              value={settings.definitionLang}
              onChange={(v) => set('definitionLang', v)}
            />
          </Row>
          <Row
            label={settings.definitionLang === 'ja' ? 'Also show English' : 'Also show Japanese'}
            hint="Show a second definition below the first"
          >
            <Toggle label={settings.definitionLang === 'ja' ? 'Also show English' : 'Also show Japanese'} value={settings.secondDefinition} onChange={(v) => set('secondDefinition', v)} />
          </Row>
          <Row label="Tap to reveal English" hint="Hide English meanings until you tap">
            <Toggle label="Tap to reveal English"
              value={settings.englishReveal === 'tap'}
              onChange={(v) => set('englishReveal', v ? 'tap' : 'show')}
            />
          </Row>
          <Row label="Furigana" hint="Show readings above kanji words on review cards" stacked>
            <Choice
              label="Furigana"
              options={FURIGANA_OPTIONS}
              value={settings.furigana}
              onChange={(v) => set('furigana', v)}
              stretch
            />
          </Row>
          <Row label="Example sentences" hint="Show Japanese examples with English translations">
            <Toggle label="Example sentences" value={settings.showExamples} onChange={(v) => set('showExamples', v)} />
          </Row>
          <Row label="Collapse mnemonics" hint="Start folded; tap the header to open one">
            <Toggle label="Collapse mnemonics" value={settings.collapseMnemonics} onChange={(v) => set('collapseMnemonics', v)} />
          </Row>
          <Row label="Mark new kanji parts" hint="Dot a mnemonic tile whose part is in none of your Known words">
            <Toggle label="Mark new kanji parts" value={settings.markNewParts} onChange={(v) => set('markNewParts', v)} />
          </Row>
        </Section>

        <Section title="Audio">
          <Row label="Auto-play" hint="Speak the card when the answer appears">
            <Toggle label="Auto-play" value={settings.autoPlayAudio} onChange={(v) => set('autoPlayAudio', v)} />
          </Row>
          <Row label="Playback source" hint="Use card recordings or your device voice" stacked>
            <Choice
              label="Playback source"
              options={CARD_AUDIO_OPTIONS}
              stretch
              value={settings.cardAudio}
              onChange={(v) => set('cardAudio', v)}
            />
          </Row>
          <Row label="Voice credit" hint="Name the voice under the audio">
            <Toggle label="Voice credit" value={settings.showVoiceCredit} onChange={(v) => set('showVoiceCredit', v)} />
          </Row>
        </Section>

        <Section title="Session">
          <Row label="New words at a time" hint="How many words the “Learn new words” button starts" stacked>
            <Choice
              label="New words at a time"
              options={NEW_PER_LEARN_OPTIONS}
              value={settings.newPerLearn}
              onChange={(v) => set('newPerLearn', v)}
              stretch
            />
          </Row>
          <Row label="Repeat missed cards" hint="Cards marked “Again” return later in the same session">
            <Toggle label="Repeat missed cards" value={settings.againRepeats} onChange={(v) => set('againRepeats', v)} />
          </Row>
          {reviewLimit.limit !== undefined ? (
            <Row
              label="Daily review limit"
              hint={reviewLimit.error ?? 'Past this, the cards you are most likely to forget come first and the rest wait'}
              stacked
            >
              <Choice
                label="Daily review limit"
                options={REVIEW_LIMIT_OPTIONS}
                value={reviewLimit.limit}
                onChange={(v) => { void reviewLimit.setLimit(v); }}
                stretch
              />
            </Row>
          ) : null}
          {vacation.loaded ? (
            <Row label="Vacation mode" hint={vacation.error ?? "Pause reviews while you're away; nothing piles up"}>
              <Toggle
                label="Vacation mode"
                value={vacation.since != null}
                onChange={(v) => { void vacation.setOn(v); }}
                disabled={vacation.settling}
              />
            </Row>
          ) : null}
        </Section>
        <Section title="Progress">
          <Pressable
            style={({ pressed }) => [styles.linkRow, pressed && { opacity: 0.7 }]}
            onPress={() => setProgressOpen(true)}
            accessibilityRole="button"
          >
            <View style={styles.linkText}>
              <Text style={styles.linkLabel}>Stats and streaks</Text>
              <Text style={styles.linkHint}>Reviews per day, words by frequency</Text>
            </View>
            <Text style={styles.chevron}>›</Text>
          </Pressable>
        </Section>

        {server.live ? <YourDataSection /> : null}

        {server.authRequired || server.signedIn ? <AccountSection /> : null}

        <Section title="Connection">
          <Row label="Status">
            <Text style={[styles.value, { color: statusColor(server.status) }]}>
              {friendlyStatus(server.status)}
            </Text>
          </Row>
          {server.isOwner ? (
            <Row label="Server" hint={server.url ? hostOf(server.url) : 'sample data'}>
              <Pressable onPress={() => setConnOpen(true)} accessibilityRole="button" accessibilityLabel="Change server address" hitSlop={8}>
                <Text style={styles.link}>Change</Text>
              </Pressable>
            </Row>
          ) : null}
          {server.status === 'offline' ? (
            <Text style={styles.dim}>Server unreachable · saved reviews may still be available</Text>
          ) : null}
          <DictionaryMissingNotice />
          {server.status !== 'online' ? (
            <Pressable
              style={({ pressed }) => [styles.retryBtn, pressed && { opacity: 0.8 }]}
              onPress={() => { void server.refresh(); }}
              accessibilityRole="button"
            >
              <Text style={styles.retryLabel}>
                {server.status === 'checking' ? 'Connecting…' : 'Retry connection'}
              </Text>
            </Pressable>
          ) : null}
        </Section>

        <Modal visible={aboutOpen} transparent animationType="slide" onRequestClose={() => setAboutOpen(false)}>
          <View style={styles.sheetBackdrop}>
            <Pressable style={StyleSheet.absoluteFill} onPress={() => setAboutOpen(false)} accessibilityLabel="Close" />
            <View style={styles.sheet}>
              <Text style={styles.sheetTitle}>Yume</Text>
              <Text style={styles.sheetLine}>App {APP_VERSION}</Text>
              <Text style={styles.sheetLine}>
                Server {server.health?.version ?? 'not connected'}
                {server.health?.commit ? ` (${server.health.commit.slice(0, 7)})` : ''}
                {server.health?.dict?.entries ? ` · ${server.health.dict.entries.toLocaleString()} dictionary entries` : ''}
              </Text>
              <Text style={styles.sheetHeading}>Data and voices</Text>
              <Text style={styles.sheetCredit}>JMdict and KANJIDIC2 © Electronic Dictionary Research and Development Group, CC BY-SA 4.0.</Text>
              <Text style={styles.sheetCredit}>Example sentences from the Tanaka Corpus via Tatoeba, CC BY 2.0 FR.</Text>
              <Text style={styles.sheetCredit}>Tokenizer dictionary: IPADIC (NAIST).</Text>
              {dictNames.length ? (
                <Text style={styles.sheetCredit}>
                  Imported dictionaries: {dictNames.join(', ')}. Wiktionary text is CC BY-SA 4.0.
                </Text>
              ) : null}
              {voices.length ? (
                <Text style={styles.sheetCredit}>Generated audio: {voiceCreditSummary(voices)}.</Text>
              ) : null}
              <View style={styles.sheetLinks}>
                {CREDIT_LINKS.map((l) => (
                  <Pressable key={l.url} onPress={() => { void Linking.openURL(l.url); }} accessibilityRole="link" hitSlop={6}>
                    <Text style={styles.sheetLink}>{l.label}</Text>
                  </Pressable>
                ))}
                {server.url ? (
                  <Pressable onPress={() => { void Linking.openURL(licensesUrl(server.url)); }} accessibilityRole="link" hitSlop={6}>
                    <Text style={styles.sheetLink}>Licenses and notices</Text>
                  </Pressable>
                ) : null}
              </View>
              <Pressable onPress={() => setAboutOpen(false)} accessibilityRole="button" hitSlop={8} style={styles.sheetClose}>
                <Text style={styles.link}>Close</Text>
              </Pressable>
            </View>
          </View>
        </Modal>

        <Modal visible={progressOpen} animationType="slide" onRequestClose={() => setProgressOpen(false)}>
        <View style={styles.progressModal}>
          <View style={styles.progressBar}>
            <Text style={styles.progressTitle}>Progress</Text>
            <Pressable onPress={() => setProgressOpen(false)} accessibilityRole="button" hitSlop={8}>
              <Text style={styles.progressClose}>Close</Text>
            </Pressable>
          </View>
          <ProgressScreen />
        </View>
      </Modal>

      <ServerConnectionModal visible={connOpen} onClose={() => setConnOpen(false)} />

        <Pressable
          style={({ pressed }) => [styles.footerRow, pressed && { opacity: 0.7 }]}
          onPress={() => setAboutOpen(true)}
          accessibilityRole="button"
          accessibilityLabel="About Yume"
        >
          <Image
            source={require('../../assets/moon.png')}
            style={styles.footerLogo}
            resizeMode="contain"
          />
          <Text style={styles.footer}>Yume</Text>
        </Pressable>
      </ScrollView>
    </View>
  );
}

/** Settings → Account: who is signed in, and sign out. Shown only when the
 *  server keeps a deck per account (or a session exists); open mode, where
 *  every request is the owner, looks as it always has. */
function AccountSection() {
  const server = useServer();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const who = server.user?.email || server.user?.display_name || 'your account';
  return (
    <Section title="Account">
      <Row label="Signed in as" hint={server.user?.role === 'owner' ? 'Server owner' : undefined}>
        <Text style={styles.valuePlain} numberOfLines={1}>{who}</Text>
      </Row>
      <Pressable
        style={({ pressed }) => [styles.retryBtn, pressed && { opacity: 0.8 }]}
        onPress={() => setConfirmOpen(true)}
        accessibilityRole="button"
      >
        <Text style={styles.retryLabel}>Sign out</Text>
      </Pressable>
      <ConfirmDialog
        visible={confirmOpen}
        title="Sign out of Yume?"
        message="Your deck stays on the server. Reviews rated offline on this device sync after you sign in again."
        confirmLabel="Sign out"
        destructive={false}
        onConfirm={() => {
          setConfirmOpen(false);
          void server.logout();
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </Section>
  );
}

/** Settings → Your data: download the deck as an Anki file, and (where the
 *  server allows it: self-hosted, not a managed host) deck snapshots. */
function YourDataSection() {
  const server = useServer();
  const client = server.client;
  const [downloading, setDownloading] = useState(false);
  const [downloadMsg, setDownloadMsg] = useState('');
  // null: the server refuses snapshots (managed host) or hasn't answered.
  const [backups, setBackups] = useState<BackupInfo[] | null>(null);
  const [maxKept, setMaxKept] = useState(3);
  const [snapBusy, setSnapBusy] = useState(false);
  const [snapMsg, setSnapMsg] = useState('');
  const [confirmSnap, setConfirmSnap] = useState(false);

  const loadBackups = useCallback(async () => {
    if (!client) return;
    try {
      setBackups(await client.listBackups());
    } catch {
      setBackups(null);
    }
  }, [client]);

  useEffect(() => {
    void loadBackups();
  }, [loadBackups]);

  const download = async () => {
    if (!client || downloading) return;
    setDownloading(true);
    setDownloadMsg('');
    try {
      const blob = await client.exportApkg();
      saveBlobOnWeb(blob, deckFileName());
      setDownloadMsg('Your deck is downloading. Open the .apkg file in Anki to import it.');
    } catch (e) {
      setDownloadMsg(`Couldn’t download your deck. ${errorText(e)}`);
    } finally {
      setDownloading(false);
    }
  };

  const snapshot = async () => {
    setConfirmSnap(false);
    if (!client || snapBusy) return;
    setSnapBusy(true);
    setSnapMsg('');
    try {
      const res = await client.createBackup('app');
      setMaxKept(res.max_kept);
      setSnapMsg(
        res.pruned?.length
          ? `Snapshot saved. The oldest one was removed to keep ${res.max_kept}.`
          : 'Snapshot saved.',
      );
      await loadBackups();
    } catch (e) {
      setSnapMsg(`Couldn’t save a snapshot. ${errorText(e)}`);
    } finally {
      setSnapBusy(false);
    }
  };

  const oldest = backups ? snapshotToPrune(backups, maxKept) : null;

  return (
    <Section title="Your data">
      <Row
        label="Download my deck"
        hint={
          Platform.OS === 'web'
            ? 'An Anki file (.apkg) with your cards, review history, tags and audio'
            : 'Open this server’s address in a browser to download your deck as an Anki file'
        }
        stacked={Platform.OS === 'web'}
      >
        {Platform.OS === 'web' ? (
          <Pressable
            style={({ pressed }) => [styles.retryBtn, pressed && { opacity: 0.8 }]}
            onPress={() => void download()}
            disabled={downloading}
            accessibilityRole="button"
          >
            <Text style={styles.retryLabel}>{downloading ? 'Preparing your deck…' : 'Download (.apkg)'}</Text>
          </Pressable>
        ) : null}
      </Row>
      {downloadMsg ? <Text style={styles.dim} accessibilityLiveRegion="polite">{downloadMsg}</Text> : null}

      {backups ? (
        <>
          <Row
            label="Snapshots"
            hint={`Copies of your deck kept on the server, newest first. Yume keeps your newest ${maxKept}. To restore one, ask ${
              server.isOwner ? 'your tutor' : 'the person who runs this server'
            }.`}
            stacked
          >
            {backups.length === 0 ? <Text style={styles.dim}>No snapshots yet.</Text> : null}
            {backups.map((b) => (
              <View key={b.name} style={styles.snapRow}>
                <Text style={styles.snapDate}>{new Date(b.created_at).toLocaleString()}</Text>
                <Text style={styles.dim}>
                  {snapshotLabel(b.name, b.kind)} · {formatBytes(b.size_bytes)}
                </Text>
              </View>
            ))}
            <Pressable
              style={({ pressed }) => [styles.retryBtn, pressed && { opacity: 0.8 }]}
              onPress={() => (oldest ? setConfirmSnap(true) : void snapshot())}
              disabled={snapBusy}
              accessibilityRole="button"
            >
              <Text style={styles.retryLabel}>{snapBusy ? 'Saving…' : 'Create snapshot'}</Text>
            </Pressable>
          </Row>
          {snapMsg ? <Text style={styles.dim} accessibilityLiveRegion="polite">{snapMsg}</Text> : null}
          <ConfirmDialog
            visible={confirmSnap}
            title="Replace the oldest snapshot?"
            message={
              oldest
                ? `Yume keeps your newest ${maxKept} snapshots. A new one deletes the oldest, from ${new Date(oldest.created_at).toLocaleString()}.`
                : undefined
            }
            confirmLabel="Create snapshot"
            onConfirm={() => void snapshot()}
            onCancel={() => setConfirmSnap(false)}
          />
        </>
      ) : null}
    </Section>
  );
}

function Choice<T extends string | number>({
  label,
  options,
  value,
  onChange,
  stretch,
}: {
  label: string;
  options: { value: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
  /** Full-width segments with centered labels — for the stacked rows. */
  stretch?: boolean;
}) {
  return (
    <View accessibilityRole="radiogroup" accessibilityLabel={label} style={[styles.segmented, stretch && styles.segmentedStretch]}>
      {options.map((o) => {
        const active = value === o.value;
        return (
          <Pressable
            key={String(o.value)}
            onPress={() => onChange(o.value)}
            style={[styles.seg, stretch && styles.segStretch, active && styles.segActive]}
            accessibilityRole="radio"
            accessibilityLabel={`${label}: ${o.label}`}
            accessibilityState={{ checked: active }}
            aria-checked={active}
          >
            <Text style={[styles.segLabel, stretch && styles.segLabelCentered, active && styles.segLabelActive]}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

function Toggle({ label, value, onChange, disabled }: { label: string; value: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <Switch
      accessibilityLabel={label}
      value={value}
      disabled={disabled}
      onValueChange={onChange}
      trackColor={{ true: colors.accent, false: colors.surface2 }}
      thumbColor={colors.textPrimary}
      // RNW's web-only ON-thumb prop (absent from RN's types); unset, it's a teal.
      {...(Platform.OS === 'web' ? ({ activeThumbColor: colors.textPrimary } as object) : null)}
    />
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View style={styles.section}>
      <Text style={styles.sectionTitle}>{title}</Text>
      <View style={styles.card}>{children}</View>
    </View>
  );
}

function Row({
  label,
  hint,
  stacked,
  children,
}: {
  label: string;
  hint?: string;
  /** Control renders full-width UNDER the text. */
  stacked?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <View style={[styles.row, stacked && styles.rowStacked]}>
      <View style={styles.rowText}>
        <Text style={styles.label}>{label}</Text>
        {hint ? <Text style={styles.hint}>{hint}</Text> : null}
      </View>
      {children}
    </View>
  );
}

function statusColor(status: string): string {
  if (status === 'online') return colors.statusKnown;
  if (status === 'checking') return colors.statusLearning;
  if (status === 'offline') return colors.statusUnknown;
  return colors.textTertiary;
}

function friendlyStatus(status: string): string {
  if (status === 'online') return 'Connected';
  if (status === 'checking') return 'Connecting…';
  if (status === 'offline') return 'Offline';
  return 'Sample mode';
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  body: { padding: spacing.lg, gap: spacing.lg },
  section: { gap: spacing.sm },
  sectionTitle: {
    fontSize: font.xs,
    color: colors.textTertiary,
    marginLeft: spacing.xs,
  },
  card: {
    backgroundColor: colors.surface1,
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    padding: spacing.md,
    gap: spacing.md,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: spacing.md,
  },
  rowStacked: {
    flexDirection: 'column',
    alignItems: 'stretch',
    gap: spacing.sm,
  },
  rowText: { flex: 1, gap: 2 },
  label: { fontSize: font.md, color: colors.textPrimary },
  hint: { fontSize: font.xs, color: colors.textTertiary },
  value: { fontSize: font.md, fontWeight: '600' },
  valuePlain: { fontSize: font.sm, color: colors.textSecondary, flexShrink: 1 },
  snapRow: { gap: 2 },
  snapDate: { fontSize: font.sm, color: colors.textPrimary },
  dim: { fontSize: font.xs, color: colors.textTertiary },
  link: { fontSize: font.sm, fontWeight: '600', color: colors.accentText },
  linkRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', minHeight: 44, gap: spacing.md },
  linkText: { flex: 1, gap: 2 },
  linkLabel: { fontSize: font.md, color: colors.textPrimary },
  linkHint: { fontSize: font.xs, color: colors.textTertiary },
  chevron: { fontSize: font.xl, color: colors.textTertiary, lineHeight: font.xl + 2 },
  sheetBackdrop: { flex: 1, backgroundColor: colors.backdrop, justifyContent: 'flex-end' },
  sheet: {
    backgroundColor: colors.surface1,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.lg,
    paddingBottom: spacing.xl,
    gap: spacing.xs,
  },
  sheetTitle: { fontFamily: fontDisplay, fontSize: font.xl, color: colors.textPrimary, marginBottom: spacing.xs },
  sheetLine: { fontSize: font.sm, color: colors.textSecondary },
  sheetHeading: { fontSize: font.sm, color: colors.textTertiary, marginTop: spacing.md },
  sheetCredit: { fontSize: font.xs, color: colors.textSecondary, lineHeight: font.xs + 6 },
  sheetLinks: { flexDirection: 'row', flexWrap: 'wrap', columnGap: spacing.md, rowGap: spacing.xs, marginTop: spacing.xs },
  sheetLink: { fontSize: font.xs, fontWeight: '600', color: colors.accentText, lineHeight: font.xs + 6 },
  sheetClose: { alignSelf: 'flex-end', marginTop: spacing.md },
  retryBtn: {
    alignSelf: 'flex-start',
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    minHeight: 38,
    justifyContent: 'center',
  },
  retryLabel: {
    fontSize: font.sm,
    color: colors.textSecondary,
    fontWeight: '600',
  },
  progressModal: { flex: 1, backgroundColor: colors.surface0 },
  progressBar: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.xl,
    paddingBottom: spacing.xs,
  },
  progressTitle: { fontSize: font.xl, fontWeight: '700', color: colors.textPrimary },
  progressClose: { fontSize: font.sm, fontWeight: '600', color: colors.accentText },
  segmented: {
    flexDirection: 'row',
    borderColor: colors.border,
    borderWidth: 1,
    borderRadius: radius.sm,
    overflow: 'hidden',
  },
  segmentedStretch: { alignSelf: 'stretch' },
  seg: { minHeight: 38, justifyContent: 'center', paddingHorizontal: spacing.sm },
  segStretch: { flex: 1 },
  segLabelCentered: { textAlign: 'center' },
  segActive: { backgroundColor: colors.accentMuted },
  segLabel: { fontSize: font.xs, color: colors.textSecondary },
  segLabelActive: { color: colors.accentText, fontWeight: '600' },
  footerRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    marginTop: spacing.md,
  },
  footerLogo: {
    width: 14,
    height: 14,
  },
  footer: {
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.sm,
    fontWeight: '600',
  },
});
