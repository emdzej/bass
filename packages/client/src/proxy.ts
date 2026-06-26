type KeyMatcher = (key: string) => boolean;

/**
 * Compile a list of pattern entries into a single matcher function.
 *   '*'                     → matches every key
 *   string with '*'         → translated to RegExp (`*` → `.*`)
 *   string without '*'      → exact match
 *   RegExp                  → used directly
 */
export function compileKeyMatcher(patterns: (string | RegExp)[] | undefined): KeyMatcher {
  if (!patterns || patterns.length === 0) return () => true;
  const regexes: RegExp[] = [];
  const exact = new Set<string>();
  let any = false;
  for (const p of patterns) {
    if (p === '*') {
      any = true;
      continue;
    }
    if (typeof p === 'string') {
      if (p.includes('*')) {
        regexes.push(globToRegex(p));
      } else {
        exact.add(p);
      }
    } else {
      regexes.push(p);
    }
  }
  if (any) return () => true;
  return (key: string) => {
    if (exact.has(key)) return true;
    for (const r of regexes) if (r.test(key)) return true;
    return false;
  };
}

function globToRegex(glob: string): RegExp {
  // Escape regex meta, then replace '*' with '.*'.
  const escaped = glob.replace(/[-/\\^$+?.()|[\]{}]/g, '\\$&').replace(/\*/g, '.*');
  return new RegExp(`^${escaped}$`);
}

export interface ProxyOptions {
  storage: Storage;
  matches: KeyMatcher;
  onWrite: (key: string, value: string | null, deleted: boolean) => void;
}

/**
 * Patch the methods of the given Storage so writes that match are forwarded
 * to onWrite. Returns a detach function that restores the originals.
 *
 * `localStorage.getItem(...)` is untouched — the proxy is write-side only;
 * the local storage is always the source of truth for reads.
 */
export function attachProxy(opts: ProxyOptions): () => void {
  const { storage, matches, onWrite } = opts;
  const origSet = storage.setItem.bind(storage);
  const origRemove = storage.removeItem.bind(storage);
  const origClear = storage.clear.bind(storage);

  storage.setItem = (key: string, value: string) => {
    origSet(key, value);
    if (matches(key)) onWrite(key, value, false);
  };
  storage.removeItem = (key: string) => {
    origRemove(key);
    if (matches(key)) onWrite(key, null, true);
  };
  // Intentionally don't intercept clear() — too blunt; if the host app
  // truly means to wipe everything we shouldn't try to sync that as
  // individual deletes.
  void origClear;

  return () => {
    storage.setItem = origSet as Storage['setItem'];
    storage.removeItem = origRemove as Storage['removeItem'];
  };
}

/**
 * Encode a string value as base64 for the wire. localStorage stores strings,
 * so this is the round-trip we use when sending writes upstream.
 */
export function encodeValue(v: string): string {
  if (typeof btoa !== 'undefined') return btoa(unescape(encodeURIComponent(v)));
  // Node fallback (vitest, etc.)
  return Buffer.from(v, 'utf-8').toString('base64');
}

/**
 * Decode a base64 wire value back to the string that goes into localStorage.
 */
export function decodeValue(b64: string): string {
  if (typeof atob !== 'undefined') return decodeURIComponent(escape(atob(b64)));
  return Buffer.from(b64, 'base64').toString('utf-8');
}
