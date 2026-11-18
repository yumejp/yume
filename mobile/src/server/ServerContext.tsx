import AsyncStorage from '@react-native-async-storage/async-storage';
import { AppState } from 'react-native';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { ApiError, YumeClient } from '../api/client';
import type { AuthUser, HealthResponse } from '../api/types';
import { CONTENT_STALE_MS, invalidateIfStale, STATS_STALE_MS } from '../data/queryStore';
import { onUnauthorized, onUnreachable } from '../lib/connectivity';
import { errorText } from '../lib/errors';

const URL_KEY = 'yume.serverUrl';
const TOKEN_KEY = 'yume.authToken';
const USER_KEY = 'yume.authUser';
const DEVUSER_KEY = 'yume.devUser';
/** The auth mode the server last reported, with its URL, so a required-mode
 *  server shows the sign-in screen at once instead of after a round of 401s. */
const SERVER_AUTH_KEY = 'yume.serverAuth';
const SESSION_ENDED = 'Your session ended. Sign in again to continue.';

/**
 * Default server for a fresh install, from EXPO_PUBLIC_YUME_SERVER_URL (keep
 * it in .env.local). Unset = sample deck. An explicit empty string stored by
 * the owner is respected.
 */
const DEFAULT_URL = process.env.EXPO_PUBLIC_YUME_SERVER_URL ?? servedFromServer();

// When the server itself serves this bundle, the API is this page's origin.
// The Metro dev server (port 8081) is not the API, so it never counts.
function servedFromServer(): string {
  if (typeof window === 'undefined' || !window.location?.origin) return '';
  if (window.location.port === '8081' || window.location.protocol === 'file:') return '';
  return window.location.origin;
}

/**
 * 'off'      — no URL configured; the app runs on the bundled sample deck.
 * 'checking' — health probe in flight.
 * 'online'   — last health probe succeeded; screens use live data.
 * 'offline'  — URL configured but unreachable; screens keep what they last
 *              loaded (Review's cached queue, Library's last words).
 */
export type ServerStatus = 'off' | 'checking' | 'online' | 'offline';

interface ServerState {
  url: string;
  setUrl: (url: string) => Promise<void>;
  status: ServerStatus;
  /** True once persisted URL and authentication state are available. */
  loaded: boolean;
  health: HealthResponse | null;
  lastError: string | null;
  client: YumeClient | null;
  /** `status === 'online'` with a client: screens use live data. */
  live: boolean;
  /** Stable (server, account) key for per-account device storage. Card ids
   *  overlap across accounts, so nothing cached under one identity may
   *  replay under another. */
  identity: string;
  refresh: () => Promise<void>;

  // --- Auth ---
  /** The signed-in account, or null when running as the server's default user. */
  user: AuthUser | null;
  /** True when requests act as the OWNER deck (signed-in role, or the
   *  X-Yume-User override; no override = owner). Gates operator-only UI. */
  isOwner: boolean;
  /** True while a stored token is present (the client sends it). */
  signedIn: boolean;
  /** The server keeps a deck per account and wants a session for each: its
   *  health says required mode, or a call without a session got 401. Open
   *  mode (the default) is never gated. */
  authRequired: boolean;
  /** Whether "Create an account" can work: the server's signup is open. */
  signupOpen: boolean;
  /** Show the sign-in screen instead of the app. */
  needsSignIn: boolean;
  /** Why the sign-in screen is showing, when a session just ended. */
  authNotice: string | null;
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string, displayName?: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Dev-only: act as a named user (e.g. "test") via X-Yume-User. null = off. */
  devUser: string | null;
  setDevUser: (name: string | null) => Promise<void>;
}

const ServerContext = createContext<ServerState | null>(null);

export function ServerProvider({ children }: { children: ReactNode }) {
  const [url, setUrlState] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [status, setStatus] = useState<ServerStatus>('off');
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [lastError, setLastError] = useState<string | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<AuthUser | null>(null);
  const [devUser, setDevUserState] = useState<string | null>(null);
  // Last auth mode the server reported, kept through offline blips so the
  // sign-in screen doesn't flicker away while the server is unreachable.
  const [serverAuth, setServerAuth] = useState<HealthResponse['auth'] | null>(null);
  const [sawUnauthorized, setSawUnauthorized] = useState(false);
  const [authNotice, setAuthNotice] = useState<string | null>(null);
  const tokenRef = useRef<string | null>(null);
  tokenRef.current = token;
  const probeSeq = useRef(0);
  const lastProbeAt = useRef(0);

  const sessionClient = useMemo(
    () => (url ? new YumeClient(url, { token, devUser }) : null),
    [url, token, devUser],
  );

  // `quiet` skips the 'checking' pass-through. Background re-probes must be
  // quiet: screens gate on status === 'online', so a loud probe flips every
  // tab to sample mode.
  const probe = useCallback(async (target: string, opts: { quiet?: boolean } = {}): Promise<ServerStatus> => {
    const seq = ++probeSeq.current;
    lastProbeAt.current = Date.now();
    if (!target) {
      setStatus('off');
      setHealth(null);
      setLastError(null);
      return 'off';
    }
    if (!opts.quiet) setStatus('checking');
    try {
      const h = await new YumeClient(target).health();
      if (probeSeq.current !== seq) return 'checking';
      setHealth(h);
      setServerAuth(h.auth ?? null);
      void AsyncStorage.setItem(SERVER_AUTH_KEY, JSON.stringify({ url: target, auth: h.auth ?? null })).catch(() => {});
      setLastError(null);
      const next: ServerStatus = h.ok ? 'online' : 'offline';
      setStatus(next);
      return next;
    } catch (err) {
      if (probeSeq.current !== seq) return 'checking';
      setHealth(null);
      setLastError(errorText(err));
      setStatus('offline');
      return 'offline';
    }
  }, []);

  // A stored URL that can't connect falls back to the default FOR THIS SESSION
  // ONLY — the stored value must survive. A stored token is cleared only on an
  // explicit auth rejection, never a network failure.
  useEffect(() => {
    (async () => {
      try {
        const [storedUrl, storedTok, storedUser, storedDev, storedAuth] = await Promise.all([
          AsyncStorage.getItem(URL_KEY),
          AsyncStorage.getItem(TOKEN_KEY),
          AsyncStorage.getItem(USER_KEY),
          AsyncStorage.getItem(DEVUSER_KEY),
          AsyncStorage.getItem(SERVER_AUTH_KEY),
        ]);
        const u = storedUrl && storedUrl.trim() ? storedUrl : DEFAULT_URL;
        if (storedAuth) {
          try {
            const last = JSON.parse(storedAuth) as { url: string; auth: HealthResponse['auth'] | null };
            if (last.url === u) setServerAuth(last.auth);
          } catch { /* ignore */ }
        }
        if (storedTok) setToken(storedTok);
        if (storedUser) {
          try { setUser(JSON.parse(storedUser) as AuthUser); } catch { /* ignore */ }
        }
        if (storedDev) setDevUserState(storedDev);
        setUrlState(u);
        setLoaded(true);

        const st = await probe(u);
        if (st === 'offline' && DEFAULT_URL && u !== DEFAULT_URL) {
          setUrlState(DEFAULT_URL);
          await probe(DEFAULT_URL);
        }
        if (storedTok && u) {
          try {
            const me = await new YumeClient(u, { token: storedTok }).me();
            setUser(me);
            await AsyncStorage.setItem(USER_KEY, JSON.stringify(me)).catch(() => {});
          } catch (err) {
            if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
              setToken(null);
              setUser(null);
              setAuthNotice(SESSION_ENDED);
              await AsyncStorage.multiRemove([TOKEN_KEY, USER_KEY]).catch(() => {});
            }
          }
        }
      } catch {
        setLoaded(true);
      }
    })();
  }, [probe]);

  // Foreground re-probe. Quiet + debounced: a loud probe flips every tab to
  // sample mode.
  useEffect(() => {
    const sub = AppState.addEventListener('change', (state) => {
      if (state === 'active' && url && Date.now() - lastProbeAt.current > 10_000) {
        void probe(url, { quiet: true });
      }
      // 'stats' drifts with time; vocab/suggested may change through another
      // client on a lazier clock.
      if (state === 'active') {
        invalidateIfStale(STATS_STALE_MS, 'stats');
        invalidateIfStale(CONTENT_STALE_MS, 'vocab', 'suggested');
      }
    });
    return () => sub.remove();
  }, [probe, url]);

  // Any request that fails to reach the server flips to 'offline' at once
  // (starting the heartbeat below); the browser's own online event ends the
  // wait early. Native has no such event, so the heartbeat covers it.
  useEffect(() => {
    const offUnreachable = onUnreachable(() => {
      setStatus((cur) => (cur === 'online' ? 'offline' : cur));
    });
    const w = typeof window !== 'undefined' && typeof window.addEventListener === 'function' ? window : null;
    const onOnline = () => {
      if (url) void probe(url, { quiet: true });
    };
    const onOffline = () => setStatus((cur) => (cur === 'online' ? 'offline' : cur));
    w?.addEventListener('online', onOnline);
    w?.addEventListener('offline', onOffline);
    return () => {
      offUnreachable();
      w?.removeEventListener('online', onOnline);
      w?.removeEventListener('offline', onOffline);
    };
  }, [probe, url]);

  // 'offline' is otherwise a trap while foregrounded (probes only run on
  // mount/foreground/save). Re-probe quietly: once fast for the VPN-wake
  // race, then on a slow heartbeat.
  useEffect(() => {
    if (status !== 'offline' || !url) return;
    const tick = () => {
      if (AppState.currentState === 'active') void probe(url, { quiet: true });
    };
    const fast = setTimeout(tick, 4_000);
    const slow = setInterval(tick, 20_000);
    return () => {
      clearTimeout(fast);
      clearInterval(slow);
    };
  }, [status, url, probe]);

  // A 401 on a call that carried the current session: it ended (expired,
  // signed out elsewhere, account removed), so drop it. A 401 with no
  // session at all means this server wants accounts even if its health
  // predates the auth field.
  useEffect(
    () =>
      onUnauthorized((sent) => {
        if (sent == null) {
          if (tokenRef.current == null) setSawUnauthorized(true);
          return;
        }
        if (sent !== tokenRef.current) return;
        tokenRef.current = null;
        setToken(null);
        setUser(null);
        setAuthNotice(SESSION_ENDED);
        void AsyncStorage.multiRemove([TOKEN_KEY, USER_KEY]).catch(() => {});
      }),
    [],
  );

  const setUrl = useCallback(
    async (next: string) => {
      const trimmed = next.trim().replace(/\/+$/, '');
      setServerAuth(null);
      setSawUnauthorized(false);
      setUrlState(trimmed);
      await AsyncStorage.setItem(URL_KEY, trimmed).catch(() => {});
      await probe(trimmed);
    },
    [probe],
  );

  const refresh = useCallback(async () => {
    await probe(url);
  }, [probe, url]);

  const applyAuth = useCallback(async (tok: string, u: AuthUser) => {
    await AsyncStorage.multiSet([
      [TOKEN_KEY, tok],
      [USER_KEY, JSON.stringify(u)],
    ]).catch(() => {});
    setToken(tok);
    setUser(u);
    setAuthNotice(null);
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      if (!url) throw new Error('Connect a server first (Settings → Connection).');
      const { token: tok, user: u } = await new YumeClient(url).login(email, password);
      await applyAuth(tok, u);
    },
    [url, applyAuth],
  );

  const signup = useCallback(
    async (email: string, password: string, displayName?: string) => {
      if (!url) throw new Error('Connect a server first (Settings → Connection).');
      const { token: tok, user: u } = await new YumeClient(url).signup(email, password, displayName);
      await applyAuth(tok, u);
    },
    [url, applyAuth],
  );

  const logout = useCallback(async () => {
    try {
      if (sessionClient) await sessionClient.logout();
    } catch {
      /* best-effort; clear locally regardless */
    }
    await AsyncStorage.multiRemove([TOKEN_KEY, USER_KEY]).catch(() => {});
    setToken(null);
    setUser(null);
  }, [sessionClient]);

  const setDevUser = useCallback(async (name: string | null) => {
    setDevUserState(name);
    if (name) await AsyncStorage.setItem(DEVUSER_KEY, name).catch(() => {});
    else await AsyncStorage.removeItem(DEVUSER_KEY).catch(() => {});
  }, []);

  const isOwner = user ? user.role === 'owner' : devUser == null || devUser === 'owner';

  // A stray 401 (a proxy in front, say) never gates a server whose health
  // says open: only one that predates the auth field is inferred from 401s.
  const authRequired =
    !!url && (serverAuth?.mode === 'required' || (serverAuth?.mode !== 'open' && sawUnauthorized));
  const signupOpen = serverAuth?.signup ?? false;
  const needsSignIn = loaded && authRequired && !token;
  // Signed out of a server that wants accounts: background syncs would only
  // collect 401s, so nothing gets a client until someone signs in.
  const client = needsSignIn ? null : sessionClient;

  // Mirrors how the server attributes requests: user id, else the X-Yume-User
  // role, else the owner deck.
  const identity = `${url}|${user?.id ?? devUser ?? 'owner'}`;

  const value = useMemo(
    () => ({
      url,
      setUrl,
      status: loaded ? status : ('off' as ServerStatus),
      loaded,
      health,
      lastError,
      client,
      live: loaded && status === 'online' && client != null,
      identity,
      refresh,
      user,
      isOwner,
      signedIn: !!token,
      authRequired,
      signupOpen,
      needsSignIn,
      authNotice,
      login,
      signup,
      logout,
      devUser,
      setDevUser,
    }),
    [url, setUrl, status, loaded, health, lastError, client, identity, refresh, user, isOwner, token, authRequired, signupOpen, needsSignIn, authNotice, login, signup, logout, devUser, setDevUser],
  );

  return <ServerContext.Provider value={value}>{children}</ServerContext.Provider>;
}

export function useServer(): ServerState {
  const ctx = useContext(ServerContext);
  if (!ctx) throw new Error('useServer must be used inside <ServerProvider>');
  return ctx;
}
