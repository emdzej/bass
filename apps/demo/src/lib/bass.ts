import { browser } from '$app/environment';
import { createBassClient, type BassClient } from '@emdzej/bass-client';

const SERVICE_URL =
  (import.meta.env.VITE_BASS_URL as string | undefined) ?? 'http://localhost:8080';
const APP_ID = (import.meta.env.VITE_APP_ID as string | undefined) ?? 'bass-demo';

let _bass: BassClient | null = null;

export function bass(): BassClient {
  if (!browser) {
    throw new Error('bass client is only available in the browser');
  }
  if (!_bass) {
    _bass = createBassClient({
      serviceUrl: SERVICE_URL,
      appId: APP_ID,
      keys: ['demo-*'],
      debounceMs: 300,
    });
  }
  return _bass;
}

export const SERVICE = SERVICE_URL;
export const APP = APP_ID;
