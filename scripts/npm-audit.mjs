// Fails on any high or critical npm advisory in mobile/ that
// docs/dependency-review.md has not already reviewed. Run from the repo root;
// npm audit reads mobile/package-lock.json, so no install is needed.
import { execFileSync } from 'node:child_process';

// Reviewed in docs/dependency-review.md: build tooling only, not in the image.
const reviewed = new Set([
  'GHSA-w3rx-r6r6-pgpr', // image-size ICNS parser loop (Metro)
  'GHSA-5p2g-fcmc-qvqq', // image-size JXL/HEIF parser loop (Metro)
  'GHSA-w5hq-g745-h8pq', // uuid v3/v5/v6 buffers (Expo config plugins, xcode)
]);

let raw;
try {
  raw = execFileSync('npm', ['audit', '--json'], { cwd: 'mobile', encoding: 'utf8', shell: process.platform === 'win32' });
} catch (e) {
  raw = e.stdout; // npm audit exits non-zero whenever it finds anything
}
const report = JSON.parse(raw);
const found = new Map();
for (const vuln of Object.values(report.vulnerabilities ?? {})) {
  for (const via of vuln.via) {
    if (typeof via !== 'object' || !['high', 'critical'].includes(via.severity)) continue;
    const id = via.url?.split('/').pop() ?? via.title;
    found.set(id, `${via.severity}: ${via.name} ${via.range}: ${via.title} (${via.url})`);
  }
}
const fresh = [...found].filter(([id]) => !reviewed.has(id));
for (const [id, line] of found) console.log(`${reviewed.has(id) ? 'reviewed' : 'NEW     '} ${line}`);
if (fresh.length) {
  console.error(`\n${fresh.length} high or critical advisory not yet reviewed. Assess it and record it in docs/dependency-review.md.`);
  process.exit(1);
}
console.log(`\nNo unreviewed high or critical advisories (${found.size} reviewed).`);
