import { render, waitFor } from '@testing-library/react-native';
import { KanjiSheet } from '../KanjiSheet';
import type { YumeClient } from '../../api/client';

it('explains the parts of a kanji inline and points out the ones still new', async () => {
  const client = {
    kanjiInfo: jest.fn().mockResolvedValue({
      kanji: '踊',
      note: 'Feet through a passage: to dance.',
      known: false,
      components: [
        { kanji: '足', meaning: 'foot', note: 'A foot with its toes.', known: true },
        { kanji: '甬', meaning: 'passage', note: 'A passage, a hollow tube.', known: false },
      ],
      words: [{ card_id: 1, lemma: '踊る', readings: ['おどる'], status: 1 }],
    }),
  } as unknown as YumeClient;
  const view = render(<KanjiSheet kanji="踊" meaning="dance" client={client} onClose={() => {}} />);
  await waitFor(() => expect(view.getByText('Feet through a passage: to dance.')).toBeTruthy());
  expect(view.getByText('Made of')).toBeTruthy();
  expect(view.getByText('A passage, a hollow tube.')).toBeTruthy();
  expect(view.getAllByText('new to you')).toHaveLength(1);
  expect(view.getByText('踊る')).toBeTruthy();
});

it('says nothing about parts when the server sends none', async () => {
  const client = {
    kanjiInfo: jest.fn().mockResolvedValue({ kanji: '足', note: 'A foot.', words: [] }),
  } as unknown as YumeClient;
  const view = render(<KanjiSheet kanji="足" client={client} onClose={() => {}} />);
  await waitFor(() => expect(view.getByText('A foot.')).toBeTruthy());
  expect(view.queryByText('Made of')).toBeNull();
});
