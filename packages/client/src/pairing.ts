import type { TokenStore } from './storage/tokens.js';
import type { PairOptions } from './types.js';

const POSTMSG_TAG = 'bass:pair-tokens';

/**
 * Build the bass /v1/pair/start URL with the given app + redirect.
 */
export function buildPairStartUrl(serviceUrl: string, appId: string, opts: PairOptions): string {
  const url = new URL(`${serviceUrl}/v1/pair/start`);
  url.searchParams.set('app_id', appId);
  url.searchParams.set('redirect_uri', opts.redirectUri);
  if (opts.deviceLabel) url.searchParams.set('device_label', opts.deviceLabel);
  if (opts.mode) url.searchParams.set('mode', opts.mode);
  return url.toString();
}

/**
 * Initiate the pairing flow. Returns a promise that resolves once tokens
 * have been captured and stored. For redirect mode, the promise never
 * resolves in the current page lifetime (page navigates away); the caller
 * should treat that case as "pairing in progress" and rely on
 * completePairingFromUrl() running in the callback route.
 */
export async function pair(
  serviceUrl: string,
  appId: string,
  tokens: TokenStore,
  opts: PairOptions,
): Promise<void> {
  const url = buildPairStartUrl(serviceUrl, appId, opts);
  const mode = opts.mode ?? 'redirect';

  if (mode === 'popup') {
    const popup = window.open(url, 'bass-pair', 'popup=yes,width=520,height=720,scrollbars=yes');
    if (!popup) {
      // popup blocked — fall back to redirect
      window.location.assign(url);
      return new Promise(() => {});
    }
    return new Promise<void>((resolve, reject) => {
      const onMsg = (ev: MessageEvent) => {
        if (ev.origin !== window.location.origin) return;
        const data = ev.data as { tag?: string; tokens?: PairTokens; error?: string };
        if (!data || data.tag !== POSTMSG_TAG) return;
        window.removeEventListener('message', onMsg);
        if (data.error) {
          reject(new Error(data.error));
          return;
        }
        if (data.tokens) {
          tokens.save({
            deviceId: data.tokens.device_id,
            syncToken: data.tokens.sync_token,
            refreshToken: data.tokens.refresh_token,
            expiresAt: Date.now() + data.tokens.expires_in * 1000,
          });
          resolve();
        }
      };
      window.addEventListener('message', onMsg);
    });
  }

  window.location.assign(url);
  return new Promise(() => {});
}

interface PairTokens {
  sync_token: string;
  refresh_token: string;
  device_id: string;
  expires_in: number;
}

/**
 * Call this once on your /sync-cb route to capture tokens from the redirect
 * fragment. Handles both flows transparently:
 *
 *  - If running inside a popup (window.opener is set and same-origin),
 *    posts tokens to the opener and closes the popup.
 *  - Otherwise stores tokens locally via the TokenStore.
 *
 * Returns true if tokens were captured.
 */
export function completePairingFromUrl(tokens: TokenStore): boolean {
  const frag = window.location.hash.replace(/^#/, '');
  if (!frag) return false;
  const params = new URLSearchParams(frag);
  const sync = params.get('sync_token');
  const refresh = params.get('refresh_token');
  const deviceId = params.get('device_id');
  const expiresIn = Number(params.get('expires_in') ?? '0');
  if (!sync || !refresh || !deviceId || !expiresIn) return false;

  const captured: PairTokens = {
    sync_token: sync,
    refresh_token: refresh,
    device_id: deviceId,
    expires_in: expiresIn,
  };

  // Strip the fragment immediately so tokens don't linger in the URL bar.
  const cleanUrl = window.location.pathname + window.location.search;
  window.history.replaceState(null, '', cleanUrl);

  // Popup flow: relay to opener and close.
  const opener = window.opener;
  if (opener && opener !== window) {
    try {
      opener.postMessage({ tag: POSTMSG_TAG, tokens: captured }, window.location.origin);
      window.close();
      return true;
    } catch {
      // fall through to local storage
    }
  }

  tokens.save({
    deviceId: captured.device_id,
    syncToken: captured.sync_token,
    refreshToken: captured.refresh_token,
    expiresAt: Date.now() + captured.expires_in * 1000,
  });
  return true;
}
