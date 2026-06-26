export { BassClient, createBassClient } from './client.js';
export { buildPairStartUrl, completePairingFromUrl } from './pairing.js';
export { compileKeyMatcher, encodeValue, decodeValue } from './proxy.js';
export { BassRestError } from './transport/rest.js';
export type {
  AuthState,
  BassClientOptions,
  BassDevice,
  DiscoveryConfig,
  HydrateOptions,
  HydrateResult,
  PairMode,
  PairOptions,
  PullResponse,
  PushResponse,
  PushResult,
  SyncItem,
} from './types.js';
