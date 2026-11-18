import { act, renderHook } from '@testing-library/react-native';
import type { YumeClient } from '../../../api/client';
import type { DueCard } from '../../../api/types';
import { useCardEdits } from '../useCardEdits';

const DUE = { id: 7, lemma: '把握', card_type: 'vocab', sentence: '状況を把握する。' } as DueCard;

function fakeClient() {
  return {
    setMnemonic: jest.fn().mockResolvedValue(true),
    addCardNote: jest.fn().mockResolvedValue({ id: 41, created_at: '2026-09-27T00:00:00Z' }),
    updateCard: jest.fn().mockResolvedValue(undefined),
  };
}

function renderEdits(client: ReturnType<typeof fakeClient> | null, live = true) {
  const patchCard = jest.fn();
  const onNoteAdded = jest.fn();
  const utils = renderHook(
    ({ c }: { c: ReturnType<typeof fakeClient> | null }) =>
      useCardEdits({ live, client: c as unknown as YumeClient | null, currentDue: DUE, patchCard, onNoteAdded }),
    { initialProps: { c: client } },
  );
  return { ...utils, patchCard, onNoteAdded };
}

it('patches the session once the server has the edit', async () => {
  const client = fakeClient();
  const { result, patchCard, onNoteAdded } = renderEdits(client);

  let ok = false;
  await act(async () => { ok = await result.current.saveEdit({ kind: 'meaning', explanation: 'Formal.' }); });
  expect(ok).toBe(true);
  expect(patchCard).toHaveBeenCalledWith(7, { explanation: 'Formal.' });

  await act(async () => { ok = await result.current.saveEdit({ kind: 'example', sentence: '把握した。', translation: '' }); });
  expect(onNoteAdded).toHaveBeenCalledWith(expect.objectContaining({ id: 41, card_id: 7, kind: 'example' }));
});

it('names the cause and patches nothing when the save fails', async () => {
  const client = fakeClient();
  client.updateCard.mockRejectedValueOnce(new Error('card 7 not found'));
  const { result, patchCard } = renderEdits(client);

  let ok = true;
  await act(async () => { ok = await result.current.saveEdit({ kind: 'meaning', explanation: 'Formal.' }); });
  expect(ok).toBe(false);
  expect(patchCard).not.toHaveBeenCalled();
  expect(result.current.error).toContain('card 7 not found');
});

it('asks for a connection instead of writing while offline', async () => {
  const client = fakeClient();
  const { result } = renderEdits(client, false);

  await act(async () => { await result.current.saveEdit({ kind: 'meaning', explanation: 'Formal.' }); });
  expect(client.updateCard).not.toHaveBeenCalled();
  expect(result.current.error).toBe('Connect to your server to save changes to this card.');
});

it('drops a save that resolves after the account changed', async () => {
  // Card ids overlap across accounts: patching the next account's session
  // with the old account's edit would show it on the wrong card.
  const client = fakeClient();
  let release!: () => void;
  client.updateCard.mockReturnValueOnce(new Promise<void>((res) => { release = res; }));
  const { result, rerender, patchCard } = renderEdits(client);

  let saving!: Promise<boolean>;
  act(() => { saving = result.current.saveEdit({ kind: 'meaning', explanation: 'Formal.' }); });
  rerender({ c: fakeClient() });

  let ok = true;
  await act(async () => {
    release();
    ok = await saving;
  });
  expect(ok).toBe(false);
  expect(patchCard).not.toHaveBeenCalled();
});
