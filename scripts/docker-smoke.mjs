// Disposable integration test: real entrypoint, downloads, HTTP API and restore.
// Run after docker build -t yume-ci .; never points at an existing installation.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import assert from 'node:assert/strict';

const prefix = `yume-smoke-${randomUUID()}`;
const source = `${prefix}-source`, restored = `${prefix}-restored`;
const originalVolume = `${prefix}-original`, restoredVolume = `${prefix}-restore`;
const copyContainer = `${prefix}-copy`;
const temp = mkdtempSync(join(tmpdir(), 'yume-smoke-'));
const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', timeout: 120000 }).trim();
const pause = ms => new Promise(r => setTimeout(r, ms));
let base;
async function ready(name) {
  const port = docker('port', name, '7654/tcp').split(':').at(-1);
  base = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 180; i++) {
    try {
      const response = await fetch(`${base}/health`, { signal: AbortSignal.timeout(2000) });
      if (response.ok && (await response.json()).ok) return;
    } catch {}
    await pause(1000);
  }
  throw new Error(`Startup failed: ${docker('logs', name)}`);
}
async function api(method, body = {}) {
  const response = await fetch(`${base}/${method}`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body), signal: AbortSignal.timeout(10000),
  });
  assert.equal(response.status, 200, `${method}: ${await response.clone().text()}`);
  return response.json();
}
// The same hardening docker-compose.yml applies. The container binds 0.0.0.0,
// so open mode needs the operator's opt-in, as docker-compose.yml gives it.
function start(name, volume) {
  docker('run', '-d', '--name', name, '-p', '127.0.0.1::7654', '-e', 'YUME_ALLOW_OPEN_NETWORK=1',
    '-v', `${volume}:/data`,
    '--read-only', '--tmpfs', '/tmp', '--cap-drop', 'ALL',
    ...['CHOWN', 'DAC_OVERRIDE', 'SETGID', 'SETUID'].flatMap(cap => ['--cap-add', cap]),
    '--security-opt', 'no-new-privileges:true', 'yume-ci');
}
// Everything in /data belongs to the unprivileged server user.
function assertOwnedByServer(name) {
  assert.equal(docker('exec', name, 'sh', '-c', 'find /data ! -user 10001 | head -n 1'), '', '/data is owned by uid 10001');
}
try {
  assert.throws(
    () => docker('run', '--rm', '--entrypoint', 'yume-server', 'yume-ci', '-bind', '0.0.0.0', '-data', '/tmp/yume'),
    e => /YUME_ALLOW_OPEN_NETWORK/.test(String(e.stderr)),
    'open mode refuses a network bind without the opt-in',
  );
  docker('volume', 'create', originalVolume);
  start(source, originalVolume);
  await ready(source);
  assert.match(await (await fetch(base)).text(), /<html/i);
  const health = await (await fetch(`${base}/health`)).json();
  assert.ok(health.version && Number.isInteger(health.api_level), `build identity in /health: ${JSON.stringify(health)}`);
  assert.match(await (await fetch(`${base}/licenses`)).text(), /NO WARRANTY/, 'license notices are served');
  assert.match(docker('exec', source, 'cat', '/usr/share/doc/yume/LICENSES.md'), /IPADIC/, 'image carries LICENSES.md');
  const lookup = await api('lookup', { queries: ['猫'] });
  assert.ok(lookup.results.some(r => r.entries?.length), 'fresh dictionary is searchable');
  const card = await api('save_card', { lemma: '猫', readings: ['ねこ'], glosses: [['cat']] });
  const media = [];
  for (const [kind, mime] of [['audio', 'audio/wav'], ['image', 'image/png']]) {
    // Opaque fixture bytes test persistence, not decoding/playback.
    const data = Buffer.from(`fixture-${kind}`).toString('base64');
    const entry = await api('add_card_media', { card_id: card.id, kind, media: { mime, data } });
    media.push({ id: entry.id, data });
  }
  await api('review_card', { card_id: card.id, rating: 3, review_id: randomUUID() });
  async function verify() {
    const status = await api('card_status', { lemmas: ['猫'] });
    assert.equal(status.lemma_statuses[0].card_id, card.id);
    assert.equal((await api('list_card_media', { card_id: card.id })).media.length, 2);
    for (const m of media) assert.equal((await api('get_card_media_file', { media_id: m.id })).data, m.data);
    return api('deck_stats');
  }
  const before = await verify();
  docker('restart', source);
  await ready(source);
  assert.deepEqual(await verify(), before, 'restart preserves study state');
  assertOwnedByServer(source);
  // A volume written by an image from before 0.1.0, which ran as root.
  docker('stop', source);
  docker('run', '--rm', '--entrypoint', 'chown', '-v', `${originalVolume}:/data`, 'yume-ci', '-R', '0:0', '/data');
  docker('start', source);
  await ready(source);
  assert.deepEqual(await verify(), before, 'a root-owned volume keeps working');
  assertOwnedByServer(source);
  docker('stop', source);
  const backup = join(temp, 'backup');
  docker('cp', `${source}:/data`, backup);
  docker('volume', 'create', restoredVolume);
  docker('create', '--name', copyContainer, '-v', `${restoredVolume}:/data`, 'yume-ci');
  docker('cp', `${backup}/.`, `${copyContainer}:/data/`);
  start(restored, restoredVolume);
  await ready(restored);
  assert.deepEqual(await verify(), before, 'restore preserves study state and media');
  assertOwnedByServer(restored);
  console.log('PASS open-mode bind guard, fresh Docker install, notices, lookup, card/media, review, restart, root-owned volume, full restore');
} finally {
  for (const name of [source, restored, copyContainer]) {
    try { docker('rm', '-f', name); } catch {}
  }
  for (const name of [originalVolume, restoredVolume]) {
    try { docker('volume', 'rm', name); } catch {}
  }
  rmSync(temp, { recursive: true, force: true });
}
