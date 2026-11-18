import { orderNotes } from '../cardNotes';

describe('orderNotes', () => {
  it('puts examples first, then tutor notes, keeping server order within a kind', () => {
    const notes = [
      { id: 1, kind: 'tutor' as const },
      { id: 2, kind: 'example' as const },
      { id: 3, kind: 'example' as const },
      { id: 4, kind: 'triage' as const },
      { id: 5, kind: 'tutor' as const },
    ];
    expect(orderNotes(notes).map((n) => n.id)).toEqual([2, 3, 1, 5, 4]);
  });
  it('does not mutate its input', () => {
    const notes = [{ id: 1, kind: 'tutor' as const }, { id: 2, kind: 'example' as const }];
    orderNotes(notes);
    expect(notes.map((n) => n.id)).toEqual([1, 2]);
  });
});
