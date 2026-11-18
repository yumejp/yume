/** Helpers for Settings → Your data: the deck download and snapshot list. */

import type { BackupInfo } from '../api/types';

/** Same name the server's Content-Disposition uses: yume-deck-2026-09-27.apkg. */
export function deckFileName(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `yume-deck-${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}.apkg`;
}

const KIND_LABELS: Record<string, string> = {
  import: 'Saved before an import',
  prerestore: 'Saved before a restore',
  premigrate: 'Saved before an update',
};

/** Why a snapshot exists: its `kind` where the server reports one, else its
 *  file name (cards-[<kind>-]20260926-212021.994[-<label>][-2].db). */
export function snapshotLabel(name: string, kind?: string): string {
  const m = name.match(/^cards-(?:(import|prerestore|premigrate)-)?\d{8}-\d{6}\.\d{3}(?:-([a-z0-9-]+?))?(?:-\d+)?\.db$/);
  const k = kind || m?.[1];
  if (k && KIND_LABELS[k]) return KIND_LABELS[k];
  const label = m?.[2] ?? '';
  if (label === 'app') return 'Saved from the app';
  if (label === 'prerestore') return 'Saved before a restore';
  if (!label) return 'Saved snapshot';
  return `Saved as “${label}”`;
}

/** The snapshot "Create snapshot" would prune, or null while there is room.
 *  Servers that report kinds cap each kind separately, so only manual
 *  snapshots count; older servers cap the whole list. `backups` is newest
 *  first, as /list_backups returns it. */
export function snapshotToPrune(backups: BackupInfo[], maxKept: number): BackupInfo | null {
  const pool = backups.some((b) => b.kind) ? backups.filter((b) => b.kind === 'manual') : backups;
  return pool.length >= maxKept ? pool[pool.length - 1] : null;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

/** Hand a downloaded file to the browser's save flow. Web only. */
export function saveBlobOnWeb(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Safari reads the URL after click() returns.
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}
