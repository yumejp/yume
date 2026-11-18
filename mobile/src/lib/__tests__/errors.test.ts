import { ApiError } from '../../api/client';
import { accountErrorText, errorText, isNetworkError, OFFLINE_TEXT } from '../errors';

const abort = () => Object.assign(new Error('Fetch is aborted'), { name: 'AbortError' });

describe('isNetworkError', () => {
  it('is true for aborted and rejected fetches only', () => {
    expect(isNetworkError(abort())).toBe(true);
    expect(isNetworkError(new TypeError('Network request failed'))).toBe(true);
    expect(isNetworkError(new ApiError('boom', 500))).toBe(false);
    expect(isNetworkError(new Error('network down'))).toBe(false);
    expect(isNetworkError('nope')).toBe(false);
  });
});

describe('accountErrorText', () => {
  it('names the cause of a failed sign-in or signup', () => {
    expect(accountErrorText(new ApiError('invalid email or password', 401), 'signin')).toMatch(/don’t match an account/);
    expect(accountErrorText(new ApiError('email already registered', 409), 'create')).toMatch(/Sign in instead/);
    expect(accountErrorText(new ApiError('new accounts are turned off on this server', 403, 'signup_closed'), 'create'))
      .toMatch(/turned off on this server/);
    expect(accountErrorText(new ApiError('too many attempts', 429), 'signin')).toMatch(/Wait a minute/);
    expect(accountErrorText(new ApiError('password must be at least 8 characters', 400), 'create')).toMatch(/at least 8/);
    expect(accountErrorText(abort(), 'signin')).toBe(OFFLINE_TEXT);
  });
});

describe('errorText', () => {
  it('never shows raw fetch wording', () => {
    expect(errorText(abort())).toBe(OFFLINE_TEXT);
    expect(errorText(new TypeError('Failed to fetch'))).toBe(OFFLINE_TEXT);
  });
  it('maps HTTP statuses to plain language', () => {
    expect(errorText(new ApiError('authentication required', 401))).toBe('You’re signed out. Sign in to continue.');
    expect(errorText(new ApiError('This operation is managed by the host.', 403))).toBe('This operation is managed by the host.');
    expect(errorText(new ApiError('lookup 502: bad gateway', 502))).toMatch(/server hit a problem/);
    expect(errorText(new ApiError('save_card 400: <html>', 400))).toBe('The server rejected that request.');
  });
  it('shows why the server is unavailable', () => {
    expect(errorText(new ApiError('dictionary not installed: run `yume-server setup -data /data`, then restart the server', 503)))
      .toBe('Dictionary not installed: run `yume-server setup -data /data`, then restart the server');
    expect(errorText(new ApiError('lookup 503: <html>busy</html>', 503))).toMatch(/server hit a problem/);
    expect(errorText(new ApiError('database is locked', 500))).toMatch(/server hit a problem/);
  });
  it('passes the server’s own message through', () => {
    expect(errorText(new ApiError('lemma is required', 400))).toBe('lemma is required');
    expect(errorText(new Error('Connect a server first.'))).toBe('Connect a server first.');
    expect(errorText(undefined)).toBe('Something went wrong.');
  });
});

