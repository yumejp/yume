import { act, render, waitFor } from '@testing-library/react-native';

// The screen reads the server/settings contexts directly — mock them at
// module level so each test sets the connection state it needs.
const mockClient = {
  listVocab: jest.fn(),
  vocabBands: jest.fn(),
};
const mockServer: {
  url: string;
  loaded: boolean;
  status: string;
  client: { listVocab: jest.Mock; vocabBands: jest.Mock } | null;
  identity: string;
  refresh: () => Promise<void>;
} = {
  url: 'http://pi:7654',
  loaded: true,
  status: 'online',
  client: mockClient,
  identity: 'test-0',
  refresh: () => Promise.resolve(),
};

jest.mock('../../server/ServerContext', () => ({
  useServer: () => mockServer,
}));
jest.mock('../../settings/SettingsContext', () => ({
  useSettings: () => ({ settings: { definitionLang: 'en' } }),
  useTextScale: () => 1,
}));

// eslint-disable-next-line import/first
import VocabScreen from '../VocabScreen';

// The first render loads the whole screen's module graph, which can take
// longer than the default 5 s on a cold, busy machine.
jest.setTimeout(30_000);

const row = (id: number, lemma: string) => ({
  card_id: id,
  lemma,
  readings: [],
  glosses: [['gloss']],
  status: 1,
  band: '1k',
});

beforeEach(() => {
  jest.clearAllMocks();
  mockServer.url = 'http://pi:7654';
  mockServer.status = 'online';
  mockServer.client = mockClient;
  mockServer.identity = 'test-0';
  mockClient.listVocab.mockResolvedValue({ rows: [row(1, '食べる'), row(2, '図書館')], total: 2 });
  mockClient.vocabBands.mockResolvedValue([]);
});

it('keeps the learner’s words when the server drops, never sample words', async () => {
  const view = render(<VocabScreen />);
  await waitFor(() => expect(view.getByText('図書館')).toBeTruthy());

  mockServer.status = 'offline';
  view.rerender(<VocabScreen />);
  await waitFor(() =>
    expect(view.getByText('Offline · showing the words loaded before the connection dropped')).toBeTruthy(),
  );
  expect(view.getByText('食べる')).toBeTruthy();
  expect(view.getByText('図書館')).toBeTruthy();
  expect(view.queryByText('冒険')).toBeNull();
});

it('treats a re-probe as live', async () => {
  const view = render(<VocabScreen />);
  await waitFor(() => expect(view.getByText('図書館')).toBeTruthy());

  mockServer.status = 'checking';
  view.rerender(<VocabScreen />);
  await act(async () => {});
  expect(view.getByText('図書館')).toBeTruthy();
  expect(view.queryByText(/Offline/)).toBeNull();
});

it('loads the other server’s words after a switch in Settings', async () => {
  const view = render(<VocabScreen />);
  await waitFor(() => expect(view.getByText('図書館')).toBeTruthy());

  // setUrl(): a new client and identity, then a loud probe (checking, online).
  let resolveB: (v: unknown) => void = () => {};
  const clientB = {
    listVocab: jest.fn(() => new Promise((r) => { resolveB = r; })),
    vocabBands: jest.fn().mockResolvedValue([]),
  };
  mockServer.url = 'http://other:7654';
  mockServer.client = clientB;
  mockServer.identity = 'test-1';
  view.rerender(<VocabScreen />);
  mockServer.status = 'checking';
  view.rerender(<VocabScreen />);
  mockServer.status = 'online';
  view.rerender(<VocabScreen />);
  await act(async () => {});

  // While the new deck loads, the old deck's cards are gone: their ids mean
  // other cards on this server.
  expect(clientB.listVocab).toHaveBeenCalled();
  expect(view.queryByText('図書館')).toBeNull();

  await act(async () => {
    resolveB({ rows: [row(1, '冒険者')], total: 1 });
  });
  await waitFor(() => expect(view.getByText('冒険者')).toBeTruthy());
  expect(view.queryByText('食べる')).toBeNull();
});

it('says the library is offline instead of showing strangers’ words', async () => {
  mockServer.status = 'offline';
  const view = render(<VocabScreen />);
  await act(async () => {});
  expect(view.getByText('Offline · your words return when the server is reachable')).toBeTruthy();
  expect(view.queryByText('冒険')).toBeNull();
  expect(view.queryByText(/library is empty/)).toBeNull();
  expect(mockClient.listVocab).not.toHaveBeenCalled();
});

it('labels the sample words when no server was ever set up', async () => {
  mockServer.url = '';
  mockServer.status = 'off';
  mockServer.client = null;
  const view = render(<VocabScreen />);
  await act(async () => {});
  expect(view.getByText('Sample words · connect your server in Settings to see your own')).toBeTruthy();
  expect(view.getByText('冒険')).toBeTruthy();
});
