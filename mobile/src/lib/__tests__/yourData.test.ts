import type { BackupInfo } from '../../api/types';
import { deckFileName, formatBytes, snapshotLabel, snapshotToPrune } from '../yourData';

describe('snapshotLabel', () => {
  it('says why each snapshot exists', () => {
    expect(snapshotLabel('cards-20260926-212021.994-app.db')).toBe('Saved from the app');
    expect(snapshotLabel('cards-20260926-212021.994-app-2.db')).toBe('Saved from the app');
    expect(snapshotLabel('cards-20260926-212021.994-prerestore.db')).toBe('Saved before a restore');
    expect(snapshotLabel('cards-20260926-212021.994-textbook-ch3.db')).toBe('Saved as “textbook-ch3”');
    expect(snapshotLabel('cards-20260926-212021.994.db')).toBe('Saved snapshot');
  });
  it('reads the kind of an automatic snapshot', () => {
    expect(snapshotLabel('cards-import-20260926-212021.994-csv-batch.db')).toBe('Saved before an import');
    expect(snapshotLabel('cards-premigrate-20260926-212021.994.db')).toBe('Saved before an update');
    expect(snapshotLabel('cards-20260926-212021.994-app.db', 'manual')).toBe('Saved from the app');
    expect(snapshotLabel('cards-20260926-212021.994-prerestore.db', 'prerestore')).toBe('Saved before a restore');
  });
});

describe('snapshotToPrune', () => {
  const snap = (name: string, kind?: string): BackupInfo => ({ name, kind, created_at: '', size_bytes: 0 });
  it('counts the whole list on servers without kinds', () => {
    const list = [snap('c'), snap('b'), snap('a')];
    expect(snapshotToPrune(list, 3)?.name).toBe('a');
    expect(snapshotToPrune(list.slice(0, 2), 3)).toBeNull();
  });
  it('counts only manual snapshots where each kind has its own cap', () => {
    const list = [snap('m3', 'manual'), snap('i2', 'import'), snap('m2', 'manual'), snap('i1', 'import'), snap('m1', 'manual')];
    expect(snapshotToPrune(list, 3)?.name).toBe('m1');
    expect(snapshotToPrune(list.filter((b) => b.name !== 'm1'), 3)).toBeNull();
  });
});

describe('formatBytes', () => {
  it('rounds to a readable unit', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2802)).toBe('3 KB');
    expect(formatBytes(5 * 1024 * 1024 + 200_000)).toBe('5.2 MB');
  });
});

describe('deckFileName', () => {
  it('matches the server’s download name', () => {
    expect(deckFileName(new Date(2026, 8, 7))).toBe('yume-deck-2026-09-07.apkg');
  });
});
