import { ApiError, isNetworkError } from '../api/client';

export { isNetworkError };

export const OFFLINE_TEXT = 'Can’t reach the server.';

/** Learner-facing text for any thrown value. Raw fetch/HTTP wording never
 *  reaches the screen. */
export function errorText(e: unknown): string {
  if (isNetworkError(e)) return OFFLINE_TEXT;
  if (e instanceof ApiError) {
    // A 401 also brings up the sign-in screen (ServerContext). A 403 falls
    // through: the server's own words say what the account may not do.
    if (e.status === 401) return 'You’re signed out. Sign in to continue.';
    if (e.status === 404) return 'Not found on the server.';
    // The client's fallback is "<method> <status>: <body>"; anything else is
    // the server's own { error } text.
    const fromServer = !/^\w+ \d{3}\b/.test(e.message);
    // A 503 names what the server is missing (e.g. its dictionary), which
    // says more than a retry hint.
    if (e.status === 503 && fromServer && e.message) {
      return e.message.charAt(0).toUpperCase() + e.message.slice(1);
    }
    if (e.status >= 500) return 'The server hit a problem. Try again in a moment.';
    if (!fromServer) return 'The server rejected that request.';
    return e.message;
  }
  if (e instanceof Error && e.message) return e.message;
  return 'Something went wrong.';
}

/** Sign-in and create-account failures, by status and code only. */
export function accountErrorText(e: unknown, action: 'signin' | 'create'): string {
  if (e instanceof ApiError) {
    if (e.code === 'signup_closed') {
      return 'New accounts are turned off on this server. Ask the person who runs it to let you join.';
    }
    if (e.status === 429) return 'Too many attempts. Wait a minute, then try again.';
    if (action === 'signin' && e.status === 401) {
      return 'That email and password don’t match an account on this server. Check them and try again.';
    }
    if (action === 'create' && e.status === 409) {
      return 'An account with this email already exists. Sign in instead.';
    }
    if (e.status === 400) {
      return 'Check your email address, and use a password of at least 8 characters.';
    }
  }
  return errorText(e);
}

