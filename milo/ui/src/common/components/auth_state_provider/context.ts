// Copyright 2024 The LUCI Authors.
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

import { createContext, useContext } from 'react';

import { AuthState } from '@/common/api/auth_state';

export interface AuthStateContextValue {
  readonly getAuthState: () => AuthState;
  readonly getAccessToken: () => Promise<string>;
  readonly getIdToken: () => Promise<string>;
}

export const AuthStateContext = createContext<AuthStateContextValue | null>(
  null,
);

/**
 * Returns the latest auth state. For ephemeral properties (e.g. ID/access
 * tokens, use the `useGet...Token` hooks instead.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 */
export function useAuthState(): Pick<
  AuthState,
  'identity' | 'email' | 'picture'
> {
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useAuthState must be used under a AuthStateProvider');
  }

  return value.getAuthState();
}

/**
 * Returns a function that resolves the latest non-expired access token of the
 * current user when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 *
 * The getter is referentially stable as long as the user identity remains the
 * same (memorized by a `useMemo` hook).
 */
export function useGetAccessToken(): () => Promise<string> {
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useGetAccessToken must be used under a AuthStateProvider');
  }

  return value.getAccessToken;
}

/**
 * Returns a function that resolves the latest non-expired ID token of the
 * current user when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 *
 * The getter is referentially stable as long as the user identity remains the
 * same (memorized by a `useMemo` hook).
 */
export function useGetIdToken(): () => Promise<string> {
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useGetIdToken must be used under a AuthStateProvider');
  }

  return value.getIdToken;
}
