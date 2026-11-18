// Builds server/internal/notices/THIRD_PARTY_NOTICES.txt: LICENSES.md, then the
// license text of every Go module linked into yume-server and every production
// npm package of the bundled app. The server embeds the file and serves it at
// /licenses; the image also ships it under /usr/share/doc/yume.
//
//   node scripts/third-party-notices.mjs          regenerate the file
//   node scripts/third-party-notices.mjs --check  fail if it is stale (CI)
//
// Both modes fail on a license outside the allow rules below (GPL family, SSPL,
// non-commercial, or unknown). Needs Go and `npm ci` in mobile/.
import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const out = join(root, 'server', 'internal', 'notices', 'THIRD_PARTY_NOTICES.txt');
const check = process.argv.includes('--check');

// Dual-licensed packages: the license Yume takes. Recorded here so the choice
// is deliberate and visible in the notices.
const ELECTIONS = { 'node-forge': 'BSD-3-Clause' };
const DENIED = /GPL|SSPL|BUSL|Commons-Clause|-NC\b|-NC-|UNLICENSED/i;
// Platforms the server is built for; linked modules can differ per target.
const TARGETS = ['linux/amd64', 'linux/arm64', 'windows/amd64', 'darwin/arm64', 'darwin/amd64'];

const problems = [];
const byName = key => (a, b) => (a[key] < b[key] ? -1 : a[key] > b[key] ? 1 : 0);
const lf = s => s.replace(/\r\n?/g, '\n').replace(/[ \t]+$/gm, '').trim();
const noticeFiles = dir => existsSync(dir)
  ? readdirSync(dir, { withFileTypes: true })
      .filter(e => e.isFile() && /^(.*[-_])?(licen[cs]e|copying|notice)([-_.].*)?$/i.test(e.name))
      .map(e => e.name).sort()
  : [];

// A small classifier for Go modules, which carry no license metadata. Anything
// it cannot name is "unknown" and fails the check until a human reviews it.
function classify(text) {
  if (/GNU (AFFERO |LESSER |LIBRARY )?GENERAL PUBLIC LICENSE/i.test(text)) return 'GPL-family';
  if (/Apache License,?\s+Version 2\.0/i.test(text)) return 'Apache-2.0';
  if (/Mozilla Public License,?\s+(Version|v\.?)\s*2\.0/i.test(text)) return 'MPL-2.0';
  if (/Permission is hereby granted, free of charge/i.test(text)) return 'MIT';
  if (/Permission to use, copy, modify, and\/or distribute/i.test(text)) return 'ISC';
  if (/Redistribution and use in source and binary forms/i.test(text)) {
    return /Neither the name|name of .{1,80} may not be used|names of its\s+contributors/i.test(text) ? 'BSD-3-Clause' : 'BSD-2-Clause';
  }
  return 'unknown';
}

function acceptable(expr, name) {
  if (!expr) return null;
  const choices = expr.replace(/[()]/g, '').split(/\s+OR\s+/i).map(s => s.trim());
  const elected = ELECTIONS[name];
  if (elected && choices.includes(elected)) return elected;
  const ok = choices.filter(c => c && c !== 'unknown' && !DENIED.test(c) && !/^SEE LICEN[CS]E/i.test(c));
  if (!ok.length) return null;
  return choices.length > 1 ? ok[0] : expr;
}

// --- Go: modules linked into the server binary on every release target ---
const serverDir = join(root, 'server');
const goModules = new Map();
for (const target of TARGETS) {
  const [GOOS, GOARCH] = target.split('/');
  const listed = execFileSync('go', ['list', '-deps', '-f', '{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}{{end}}', '.'],
    { cwd: serverDir, encoding: 'utf8', env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: '0' } });
  for (const line of listed.split('\n').filter(Boolean)) {
    const [path, version, dir] = line.split('\t');
    goModules.set(`${path}@${version}`, { path, version, dir });
  }
}
const goRoot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim();
const goEntries = [{ name: 'Go standard library and runtime', id: 'BSD-3-Clause', texts: [lf(readFileSync(join(goRoot, 'LICENSE'), 'utf8'))] }];
for (const m of [...goModules.values()].sort(byName('path'))) {
  const files = noticeFiles(m.dir);
  if (!files.length) problems.push(`Go module ${m.path} ${m.version} has no license file`);
  const texts = files.map(f => `${files.length > 1 ? `--- ${f} ---\n` : ''}${lf(readFileSync(join(m.dir, f), 'utf8'))}`);
  const main = files.find(f => /^(licen[cs]e|copying)(\.(md|txt))?$/i.test(f)) ?? files[0];
  const id = main ? classify(readFileSync(join(m.dir, main), 'utf8')) : 'unknown';
  if (!acceptable(id, m.path)) problems.push(`Go module ${m.path} ${m.version}: license ${id}`);
  goEntries.push({ name: `${m.path} ${m.version}`, id, texts });
}

// --- npm: production packages of the app bundled into the server ---
const mobileDir = join(root, 'mobile');
const lock = JSON.parse(readFileSync(join(mobileDir, 'package-lock.json'), 'utf8'));
const npmEntries = [], platformOnly = [];
const seen = new Set();
for (const [key, pkg] of Object.entries(lock.packages)) {
  if (!key || pkg.dev || pkg.devOptional || pkg.link) continue;
  const name = key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length);
  const label = `${name} ${pkg.version}`;
  if (seen.has(label)) continue;
  seen.add(label);
  const declared = typeof pkg.license === 'string' ? pkg.license : '';
  const id = acceptable(declared, name);
  if (!id) problems.push(`npm package ${label}: license "${declared || 'none declared'}"`);
  const shown = id && id !== declared ? `${id} (elected from ${declared})` : declared || 'none declared';
  // Prebuilt per-platform binaries exist only where they install; list them
  // by name so the file is the same on every machine.
  if (pkg.os || pkg.cpu) {
    platformOnly.push(`${label}: ${shown}`);
    continue;
  }
  const dir = join(mobileDir, key);
  if (!existsSync(dir)) throw new Error(`${key} is not installed; run npm ci in mobile/ first`);
  const files = noticeFiles(dir);
  npmEntries.push({ name: label, id: shown, texts: files.map(f => `${files.length > 1 ? `--- ${f} ---\n` : ''}${lf(readFileSync(join(dir, f), 'utf8'))}`) });
}
npmEntries.sort(byName('name'));
platformOnly.sort();

// Identical license texts are printed once, under every package that uses them.
function section(title, entries) {
  const groups = new Map();
  for (const e of entries) {
    const body = e.texts.length ? e.texts.join('\n\n') : '(The package ships no license file; its package metadata declares the license above.)';
    const k = `${e.id}\n${body}`;
    if (!groups.has(k)) groups.set(k, { id: e.id, body, names: [] });
    groups.get(k).names.push(e.name);
  }
  const rule = '='.repeat(78);
  return [`${rule}\n${title}\n${rule}`, ...[...groups.values()].map(g =>
    `${'-'.repeat(78)}\n${g.names.join('\n')}\nLicense: ${g.id}\n\n${g.body}`)].join('\n\n');
}

const text = [
  'Yume third-party notices',
  '',
  'Generated by scripts/third-party-notices.mjs from LICENSES.md, the Go modules',
  'linked into yume-server, and the production npm packages of the bundled app.',
  'Do not edit by hand.',
  '',
  lf(readFileSync(join(root, 'LICENSES.md'), 'utf8')),
  '',
  section('Go modules in yume-server', goEntries),
  '',
  section('npm packages in the bundled app', npmEntries),
  '',
  `${'-'.repeat(78)}\nPlatform-specific build binaries (same licenses as their main packages):\n${platformOnly.join('\n')}`,
  '',
].join('\n');

if (problems.length) {
  console.error(`Licenses that need review:\n  ${problems.join('\n  ')}`);
  process.exit(1);
}
if (check) {
  const current = existsSync(out) ? readFileSync(out, 'utf8').replace(/\r\n/g, '\n') : '';
  if (current !== text) {
    console.error('server/internal/notices/THIRD_PARTY_NOTICES.txt is out of date: run node scripts/third-party-notices.mjs and commit the result.');
    process.exit(1);
  }
  console.log(`Notices current: ${goEntries.length} Go entries, ${npmEntries.length + platformOnly.length} npm packages.`);
} else {
  writeFileSync(out, text);
  console.log(`Wrote ${out} (${Math.round(text.length / 1024)} KB).`);
}
