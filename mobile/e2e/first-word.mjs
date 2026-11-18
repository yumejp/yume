// Run after `npx expo export --platform web`. Uses fixture data only, unless
// YUME_BASE points at a real Yume server with an empty deck (CI runs the
// journey against the Docker image this way).
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { readFile, mkdir } from 'node:fs/promises';
import { resolve, extname, sep } from 'node:path';
import assert from 'node:assert/strict';

const root = resolve('dist');
const external = process.env.YUME_BASE?.replace(/\/$/, '');
let saved = false;
let reviewed = false;
let deletes = 0;
const word = { id: 1, kanji: ['気になる'], readings: ['きになる'], senses: [{ glosses: ['to be curious about'], pos: [] }] };
const card = { id: 1, lemma: '気になる', readings: ['きになる'], glosses: [['to be curious about']], card_type: 'vocab', is_new: true };
const stats = () => ({ total: +saved, due: saved && !reviewed ? 1 : 0, due_reviews: 0,
  new_total: saved && !reviewed ? 1 : 0, new_introduced_today: 0 });
const server = createServer(async (req, res) => {
  const path = new URL(req.url, 'http://localhost').pathname;
  if (req.method === 'POST') {
    let raw = ''; for await (const chunk of req) raw += chunk;
    const body = JSON.parse(raw || '{}');
    let result = { ok: true };
    switch (path) {
      case '/health': result = { ok: true, version: 'fixture', dict: { loaded: true, entries: 1 } }; break;
      case '/deck_stats': result = stats(); break;
      case '/stats': result = { totals: { cards: +saved, known: 0, learning: +saved, ignored: 0, due_now: 0 }, streak: { current: 0, longest: 0 }, this_week: { known_added: 0, reviews: +reviewed, mined: +saved }, heatmap: [], due_forecast: [] }; break;
      case '/lookup': result = { results: [{ query: '気になる', entries: [word] }] }; break;
      case '/card_status': result = { statuses: [], lemma_statuses: [] }; break;
      case '/save_card': saved = true; result = { id: 1 }; break;
      case '/delete_card': deletes++; saved = false; break;
      case '/get_due_cards': result = { cards: saved && !reviewed && body.new_limit > 0 ? [card] : [] }; break;
      case '/review_card': reviewed = true; result = { card_id: 1, new_state: 2, new_due: '' }; break;
      case '/list_vocab': result = { rows: [], total: 0 }; break;
      case '/list_watch_words': result = { words: [] }; break;
      case '/list_card_notes': result = { notes: [] }; break;
      case '/list_card_media': result = { media: [] }; break;
      case '/examples': result = { examples: [] }; break;
      case '/tokenize': result = { tokens: [] }; break;
      case '/vocab_bands': result = { bands: [] }; break;
    }
    res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(result)); return;
  }
  try {
    const target = resolve(root, '.' + (path === '/' ? '/index.html' : path));
    if (!target.startsWith(root + sep)) throw new Error('invalid path');
    res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.png': 'image/png' })[extname(target)] ?? 'application/octet-stream');
    res.end(await readFile(target));
  } catch { res.writeHead(404); res.end(); }
});
// What the server holds: the fixture's flags, or the real deck through the API.
async function api(method, body = {}) {
  const response = await fetch(`${external}/${method}`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  });
  assert.equal(response.status, 200, `${method}: ${await response.clone().text()}`);
  return response.json();
}
const isSaved = async () => external
  ? (await api('card_status', { lemmas: ['気になる'] })).lemma_statuses?.some(r => r.card_id) ?? false
  : saved;
const isReviewed = async () => external ? (await api('stats')).this_week.reviews > 0 : reviewed;
const eventually = async check => {
  for (let i = 0; i < 20 && !(await check()); i++) await new Promise(r => setTimeout(r, 250));
  return check();
};
if (!external) await new Promise(r => server.listen(0, '127.0.0.1', r));
let browser;
try {
  browser = await chromium.launch({ channel: process.env.YUME_BROWSER_CHANNEL || undefined, headless: true });
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.goto(external ?? `http://127.0.0.1:${server.address().port}`);
  await page.getByRole('button', { name: 'Find a word', exact: true }).waitFor();
  await page.waitForTimeout(500);
  await mkdir('e2e/artifacts', { recursive: true });
  await page.screenshot({ path: 'e2e/artifacts/first-word.png' });
  await page.getByRole('button', { name: 'Find a word', exact: true }).click();
  await page.getByRole('textbox', { name: 'Search dictionary', exact: true }).fill('気になる');
  await page.getByRole('button', { name: 'Add to deck', exact: true }).click();
  await page.getByText('気になる is in your library.', { exact: true }).waitFor();
  assert.equal(await page.getByText('rare', { exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'In library', exact: true }).count(), 0);
  await page.getByRole('button', { name: 'Remove 気になる from deck', exact: true }).click();
  await page.getByText('Remove from deck?', { exact: true }).waitFor();
  assert.equal(deletes, 0);
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  assert.equal(await isSaved(), true);
  await page.waitForTimeout(400);
  await page.screenshot({ path: 'e2e/artifacts/saved-word.png' });
  await page.getByRole('button', { name: 'Remove 気になる from deck', exact: true }).click();
  await page.getByText(/Review history is kept/).waitFor();
  await page.getByRole('button', { name: 'Remove', exact: true }).click();
  await page.getByRole('button', { name: 'Add to deck', exact: true }).waitFor();
  if (!external) assert.equal(deletes, 1);
  assert.equal(await isSaved(), false);
  await page.getByRole('button', { name: 'Add to deck', exact: true }).click();
  await page.getByRole('button', { name: 'Go to Review', exact: true }).click();
  await page.getByRole('button', { name: 'Learn 1 new word', exact: true }).click();
  // A real server adds the next interval to the rating's name ("Good 1d").
  await page.getByRole('button', { name: /^Good\b/ }).waitFor();
  await page.waitForTimeout(600);
  await page.screenshot({ path: 'e2e/artifacts/first-review.png' });
  await page.getByRole('button', { name: /^Good\b/ }).click();
  assert.equal(await eventually(isReviewed), true);
  await page.reload();
  await page.getByText('All caught up', { exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'Find a word', exact: true }).count(), 0);
  await page.getByRole('tab', { name: 'Settings', exact: true }).click();
  await page.getByRole('switch', { name: 'Repeat missed cards', exact: true }).waitFor();
  await page.screenshot({ path: 'e2e/artifacts/settings.png' });
  await page.getByRole('radio', { name: 'Playback source: Device voice', exact: true }).click();
  await page.getByRole('radio', { name: 'Playback source: Device voice', checked: true }).waitFor();
  await page.screenshot({ path: 'e2e/artifacts/settings-audio.png' });
  assert.deepEqual(errors, []);
  console.log('PASS empty library → dictionary → save/remove → learn → first review → settings');
} finally { await browser?.close(); if (server.listening) await new Promise(r => server.close(r)); }
