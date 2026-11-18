import { fireEvent, render, waitFor } from '@testing-library/react-native';
import type { DictEntry } from '../../api/types';

// The screen reads the server/settings contexts directly — mock them at
// module level so the test drives a fake client with no providers.
const mockClient = {
  lookup: jest.fn(),
  searchEnglish: jest.fn(),
  cardStatus: jest.fn(),
  listWatchWords: jest.fn(),
};
const mockServer = { live: true, client: mockClient };

jest.mock('../../server/ServerContext', () => ({
  useServer: () => mockServer,
}));
jest.mock('../../settings/SettingsContext', () => ({
  useSettings: () => ({ settings: { definitionLang: 'en' } }),
  useTextScale: () => 1,
}));

// eslint-disable-next-line import/first
import DictionaryScreen from '../DictionaryScreen';

// The first render loads the whole screen's module graph, which can take
// longer than the default 5 s on a cold, busy machine.
jest.setTimeout(30_000);

const entry = (id: number, kanji: string, reading: string, gloss: string): DictEntry =>
  ({ id, kanji: [kanji], readings: [reading], senses: [{ glosses: [gloss], pos: [] }] }) as DictEntry;

beforeEach(() => {
  jest.clearAllMocks();
  mockClient.cardStatus.mockResolvedValue({ statuses: [], lemma_statuses: [] });
  mockClient.listWatchWords.mockResolvedValue([]);
});

it('finds English words in the default mode, after any Japanese hits', async () => {
  mockClient.lookup.mockResolvedValue([{ query: 'study', entries: [] }]);
  mockClient.searchEnglish.mockResolvedValue([entry(1, '勉強', 'べんきょう', 'study')]);
  const view = render(<DictionaryScreen />);

  fireEvent.changeText(view.getByLabelText('Search dictionary'), 'study');
  await waitFor(() => expect(view.getByText('勉強')).toBeTruthy());
  expect(mockClient.searchEnglish).toHaveBeenCalledWith('study');
  expect(view.getByText('English meaning matches')).toBeTruthy();
  // The garbled romaji reading ("sつdy") is never shown.
  expect(view.queryByText(/Searching/)).toBeNull();
});

it('lists romaji hits first and does not repeat them among meaning matches', async () => {
  const made = entry(2, '迄', 'まで', 'until');
  mockClient.lookup.mockResolvedValue([{ query: 'まで', entries: [made] }]);
  mockClient.searchEnglish.mockResolvedValue([entry(3, '作る', 'つくる', 'to make; made'), made]);
  const view = render(<DictionaryScreen />);

  fireEvent.changeText(view.getByLabelText('Search dictionary'), 'made');
  await waitFor(() => expect(view.getByText('作る')).toBeTruthy());
  expect(mockClient.lookup).toHaveBeenCalledWith(['まで']);
  expect(view.getByText(/Searching/)).toBeTruthy(); // the まで hint
  expect(view.getAllByText('迄')).toHaveLength(1);
  expect(view.getByText('2 entries')).toBeTruthy();
});

it('searches kana input only as Japanese', async () => {
  mockClient.lookup.mockResolvedValue([{ query: 'ねこ', entries: [entry(4, '猫', 'ねこ', 'cat')] }]);
  const view = render(<DictionaryScreen />);

  fireEvent.changeText(view.getByLabelText('Search dictionary'), 'ねこ');
  await waitFor(() => expect(view.getByText('猫')).toBeTruthy());
  expect(mockClient.searchEnglish).not.toHaveBeenCalled();
  expect(view.queryByText('English meaning matches')).toBeNull();
});
