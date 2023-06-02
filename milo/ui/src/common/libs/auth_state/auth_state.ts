// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { get as kvGet, set as kvSet } from 'idb-keyval';

export const ANONYMOUS_IDENTITY = 'anonymous:anonymous';

export interface AuthState {
  readonly identity: string;
  readonly email?: string;
  readonly picture?: string;
  readonly accessToken?: string;
  readonly idToken?: string;
  /**
   * Expiration time (unix timestamp) of the access token.
   *
   * If zero/undefined, the access token does not expire.
   */
  readonly accessTokenExpiry?: number;
  /**
   * Expiration time (unix timestamp) of the ID token.
   *
   * If zero/undefined, the ID token does not expire.
   */
  readonly idTokenExpiry?: number;
}

const AUTH_STATE_KEY = 'auth-state-v2';

// In-memory cache. Can be used to access the cache synchronously.
let cachedAuthState: AuthState | null = null;

/**
 * Update the auth state in IndexDB and the in-memory cache.
 */
export function setAuthStateCache(authState: AuthState | null): Promise<void> {
  cachedAuthState = authState;
  return kvSet(AUTH_STATE_KEY, authState);
}

/**
 * Gets auth state synchronously. Returns null if
 * 1. the auth state is not cached in memory, or
 * 2. the auth state has expired.
 */
export function getAuthStateCacheSync() {
  if (!cachedAuthState?.accessTokenExpiry) {
    return cachedAuthState;
  }
  return cachedAuthState.accessTokenExpiry * 1000 > Date.now()
    ? cachedAuthState
    : null;
}

/**
 * Gets auth state and populate the in-memory cache. Returns null if
 * 1. the auth state is not cached in IndexDB, or
 * 2. the auth state has expired.
 */
export async function getAuthStateCache() {
  cachedAuthState = (await kvGet<AuthState | null>(AUTH_STATE_KEY)) || null;
  return getAuthStateCacheSync();
}

/**
 * Gets the auth state associated with the current section from the server.
 *
 * Also populates the cached auth state up on successfully retrieving the auth
 * state.
 */
export async function queryAuthState(fetchImpl = fetch): Promise<AuthState> {
  const res = await fetchImpl('/auth/openid/state');
  if (!res.ok) {
    throw new Error('failed to get auth state:\n' + (await res.text()));
  }

  return res.json();
}

/**
 * Obtains a current auth state.
 * Use the cached auth state if it's not expired yet.
 * Refresh the cached auth state otherwise.
 */
export async function obtainAuthState() {
  let authState = await getAuthStateCache();
  if (authState) {
    return authState;
  }

  authState = await queryAuthState();
  setAuthStateCache(authState);

  return authState;
}
