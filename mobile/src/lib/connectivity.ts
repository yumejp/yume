/** Fan-out for "a request just failed to reach the server", so the connection
 *  state can flip to offline without waiting for the next health probe. */
type Listener = () => void;
const listeners = new Set<Listener>();

export function onUnreachable(fn: Listener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

export function reportUnreachable(): void {
  listeners.forEach((fn) => fn());
}

/** Fan-out for "the server answered 401", so the account state can drop an
 *  ended session or show the sign-in screen from wherever the call was made.
 *  `token` is the session the request carried (null: none). */
type UnauthorizedListener = (token: string | null) => void;
const unauthorizedListeners = new Set<UnauthorizedListener>();

export function onUnauthorized(fn: UnauthorizedListener): () => void {
  unauthorizedListeners.add(fn);
  return () => {
    unauthorizedListeners.delete(fn);
  };
}

export function reportUnauthorized(token: string | null): void {
  unauthorizedListeners.forEach((fn) => fn(token));
}

