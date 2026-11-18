import type { YumeClient } from '../../api/client';
import type { DueCard } from '../../api/types';
import { writeCardEdit } from '../cardEdits';

const DUE: DueCard = {
  id: 7,
  lemma: '把握',
  card_type: 'vocab',
  sentence: '状況を把握する。',
  mnemonic: {
    parts: [{ kanji: '把', meaning: 'grip', known: true }, { kanji: '握', meaning: 'clench' }],
    story: 'Shared story.',
  },
  mnemonic_shared: true,
};

function fakeClient() {
  return {
    setMnemonic: jest.fn().mockResolvedValue(true),
    addCardNote: jest.fn().mockResolvedValue({ id: 41, created_at: '2026-09-27T00:00:00Z' }),
    updateCard: jest.fn().mockResolvedValue(undefined),
  };
}

it('writes the mnemonic story as the card’s own, keeping the breakdown', async () => {
  const client = fakeClient();
  const result = await writeCardEdit(client as unknown as YumeClient, DUE, { kind: 'mnemonic', story: '  Grip it tight.  ' });

  // Served-only `known` never goes back to the server.
  expect(client.setMnemonic).toHaveBeenCalledWith(7, [{ kanji: '把', meaning: 'grip' }, { kanji: '握', meaning: 'clench' }], 'Grip it tight.');
  expect(result.patch).toEqual({ mnemonic: { parts: DUE.mnemonic!.parts, story: 'Grip it tight.' }, mnemonic_shared: false });
});

it('writes a story-only mnemonic when the card has no breakdown', async () => {
  const client = fakeClient();
  await writeCardEdit(client as unknown as YumeClient, { ...DUE, mnemonic: null }, { kind: 'mnemonic', story: 'Mine.' });
  expect(client.setMnemonic).toHaveBeenCalledWith(7, [], 'Mine.');
});

it('adds an example note on one line with its translation as a field', async () => {
  const client = fakeClient();
  const result = await writeCardEdit(client as unknown as YumeClient, DUE, {
    kind: 'example',
    sentence: '状況を\n把握した。',
    translation: ' I grasped the situation. ',
  });

  expect(client.addCardNote).toHaveBeenCalledWith(7, 'example', '状況を把握した。', 'I grasped the situation.');
  expect(result.note).toEqual({
    id: 41,
    card_id: 7,
    kind: 'example',
    note: '状況を把握した。',
    translation: 'I grasped the situation.',
    created_at: '2026-09-27T00:00:00Z',
  });
});

it('sends an example without English as the sentence alone', async () => {
  const client = fakeClient();
  await writeCardEdit(client as unknown as YumeClient, DUE, { kind: 'example', sentence: '把握する。', translation: '  ' });
  expect(client.addCardNote).toHaveBeenCalledWith(7, 'example', '把握する。', undefined);
});

it('updates the translation and explanation as card fields; empty clears', async () => {
  const client = fakeClient();
  const result = await writeCardEdit(client as unknown as YumeClient, DUE, {
    kind: 'meaning',
    translation: 'I grasp the situation.',
    explanation: ' ',
  });

  expect(client.updateCard).toHaveBeenCalledWith(7, { translation: 'I grasp the situation.', explanation: '' });
  expect(result.patch).toEqual({ translation: 'I grasp the situation.', explanation: null });
});

it('leaves the translation alone when the edit has none', async () => {
  const client = fakeClient();
  const result = await writeCardEdit(client as unknown as YumeClient, DUE, { kind: 'meaning', explanation: 'Formal.' });

  expect(client.updateCard).toHaveBeenCalledWith(7, { explanation: 'Formal.' });
  expect(result.patch).toEqual({ explanation: 'Formal.' });
});
