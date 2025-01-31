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

import { useContext } from 'react';

import { AuthState } from '@/common/api/auth_state';

import { AuthStateContext } from './auth_state_provider';

/**
 * Returns the latest auth state. For ephemeral properties (e.g. ID/access
 * tokens), use the `useGetAuthToken` hook instead.
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
    throw new Error('useAuthState can only be used in a AuthStateProvider');
  }

  return value.getAuthState();
}

export enum TokenType {
  Id = 'id',
  Access = 'access',
}

/**
 * Returns a function that resolves the latest non-expired ID or access token of
 * the current user when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 *
 * The getter is referentially stable for each token type as long as the user
 * identity remains the same (memorized by a `useMemo` hook).
 */
export function useGetAuthToken(tokenType: TokenType): () => Promise<string> {
  const value = useContext(AuthStateContext);

  if (value === undefined) {
    throw new Error('useGetAuthToken can only be used in a AuthStateProvider');
  }

  return tokenType === TokenType.Id ? value.getIdToken : value.getAccessToken;
}
