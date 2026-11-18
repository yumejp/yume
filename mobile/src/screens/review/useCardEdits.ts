import { useEffect, useRef, useState } from 'react';
import type { YumeClient } from '../../api/client';
import type { CardNote, DueCard } from '../../api/types';
import { invalidate } from '../../data/queryStore';
import { writeCardEdit, type CardEdit } from '../../lib/cardEdits';
import { errorText } from '../../lib/errors';

/** The learner's own writes on the current card, applied to the session once
 *  the server has them. */
export function useCardEdits({
  live,
  client,
  currentDue,
  patchCard,
  onNoteAdded,
}: {
  live: boolean;
  client: YumeClient | null;
  currentDue: DueCard | null;
  patchCard: (id: number, patch: Partial<DueCard>) => void;
  onNoteAdded: (note: CardNote) => void;
}): {
  saveEdit: (edit: CardEdit) => Promise<boolean>;
  saving: boolean;
  error: string | null;
  clearError: () => void;
} {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Card ids are per account: a save that resolves after a switch must not
  // patch the next account's session.
  const clientRef = useRef(client);
  useEffect(() => {
    clientRef.current = client;
    setError(null);
  }, [client]);

  const saveEdit = async (edit: CardEdit): Promise<boolean> => {
    if (!live || !client || !currentDue || currentDue.id < 0) {
      setError('Connect to your server to save changes to this card.');
      return false;
    }
    const id = currentDue.id;
    setSaving(true);
    setError(null);
    try {
      const { patch, note } = await writeCardEdit(client, currentDue, edit);
      if (clientRef.current !== client) return false;
      if (patch) patchCard(id, patch);
      if (note) onNoteAdded(note);
      invalidate('vocab');
      return true;
    } catch (e) {
      setError(`Couldn’t save to this card. ${errorText(e)}`);
      return false;
    } finally {
      setSaving(false);
    }
  };

  return { saveEdit, saving, error, clearError: () => setError(null) };
}
