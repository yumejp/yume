import { render, screen } from '@testing-library/react-native';
import type { HealthResponse } from '../../api/types';
import { DictionaryMissingNotice, missingDictionary, setupCommand } from '../DictionaryMissingNotice';

const NOT_INSTALLED =
  'dictionary not installed: run `yume-server setup -data /srv/yume` (or set YUME_JMDICT), then restart the server';

const health = (dict: HealthResponse['dict']): HealthResponse => ({ ok: true, version: '0.0.1', dict });

let mockServer: { status: string; health: HealthResponse | null; isOwner: boolean };
jest.mock('../../server/ServerContext', () => ({ useServer: () => mockServer }));

describe('missingDictionary', () => {
  it('reports only a connected server without a working dictionary', () => {
    const missing = { loaded: false, entries: 0, error: NOT_INSTALLED };
    expect(missingDictionary('online', health(missing))).toEqual(missing);
    expect(missingDictionary('online', health({ loaded: true, entries: 200000, error: '' }))).toBeNull();
    expect(missingDictionary('offline', health(missing))).toBeNull();
    expect(missingDictionary('online', { ok: true, version: 'old' })).toBeNull();
    expect(missingDictionary('online', null)).toBeNull();
  });
});

describe('setupCommand', () => {
  it('extracts the command from the not-installed error only', () => {
    expect(setupCommand(NOT_INSTALLED)).toBe('yume-server setup -data /srv/yume');
    expect(setupCommand('gunzip jmdict: gzip: invalid header')).toBeNull();
  });
});

describe('DictionaryMissingNotice', () => {
  it('gives the owner the setup command', () => {
    mockServer = { status: 'online', health: health({ loaded: false, entries: 0, error: NOT_INSTALLED }), isOwner: true };
    render(<DictionaryMissingNotice />);
    expect(screen.getByText('Dictionary not installed')).toBeTruthy();
    expect(screen.getByText('yume-server setup -data /srv/yume')).toBeTruthy();
  });

  it('tells other accounts who can fix it', () => {
    mockServer = { status: 'online', health: health({ loaded: false, entries: 0, error: NOT_INSTALLED }), isOwner: false };
    render(<DictionaryMissingNotice />);
    expect(screen.getByText(/Ask the person who runs your Yume server/)).toBeTruthy();
    expect(screen.queryByText('yume-server setup -data /srv/yume')).toBeNull();
  });

  it('names a damaged file separately', () => {
    const error = 'gunzip jmdict: gzip: invalid header';
    mockServer = { status: 'online', health: health({ loaded: false, entries: 0, error }), isOwner: true };
    render(<DictionaryMissingNotice />);
    expect(screen.getByText('Dictionary failed to load')).toBeTruthy();
    expect(screen.getByText(error)).toBeTruthy();
  });

  it('renders nothing while the dictionary works', () => {
    mockServer = { status: 'online', health: health({ loaded: true, entries: 200000, error: '' }), isOwner: true };
    render(<DictionaryMissingNotice />);
    expect(screen.queryByText(/Dictionary/)).toBeNull();
  });
});
