// Copyright 2021 The LUCI Authors.
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

import { AuthState } from './services/milo_internal';

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
 * 2. the auth state is not cached in IndexDB, or
 * 3. the auth state has expired.
 */
export function getAuthStateCacheSync() {
  if (!cachedAuthState?.accessTokenExpiry) {
    return cachedAuthState;
  }
  return cachedAuthState.accessTokenExpiry * 1000 > Date.now() ? cachedAuthState : null;
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
