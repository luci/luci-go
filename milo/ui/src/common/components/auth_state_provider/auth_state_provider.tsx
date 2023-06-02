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

import { untracked } from 'mobx';
import { observer } from 'mobx-react-lite';
import { createContext, useCallback, useContext, useState } from 'react';

import { AuthState } from '@/common/libs/auth_state';
import { useStore } from '@/common/store';

const AuthStateContext = createContext<(() => AuthState) | null>(null);

export interface AuthStateProviderProps {
  readonly initialValue: AuthState;
  readonly children: React.ReactNode;
}

export const AuthStateProvider = observer(
  ({ initialValue, children }: AuthStateProviderProps) => {
    const store = useStore();
    // Use `useState` instead of `useEffect` to ensure that the auth state is
    // initialized before constructing the `getAuthState` callback below.
    useState(() => store.authState.init(initialValue));

    const getAuthState = useCallback(
      // Use `untracked` to prevent token refresh triggering updates.
      () =>
        untracked(() => {
          // Safe to cast here because the auth state has been initialized above.
          return store.authState.value!;
        }),
      // Establish a dependency on user identity so the provided getter is
      // refreshed whenever the identity changed.
      [store.authState.identity]
    );

    return (
      <AuthStateContext.Provider value={getAuthState}>
        {children}
      </AuthStateContext.Provider>
    );
  }
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
  const getAuthState = useContext(AuthStateContext);

  if (!getAuthState) {
    throw new Error('useAuthState must be used within AuthStateProvider');
  }

  return getAuthState();
}

/**
 * Returns a function that returns the latest access token when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 */
export function useGetAccessToken(): () => string {
  const getAuthState = useContext(AuthStateContext);

  if (!getAuthState) {
    throw new Error('useGetAccessToken must be used within AuthStateProvider');
  }

  return () => getAuthState().accessToken || '';
}

/**
 * Returns a function that returns the latest ID token when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 */
export function useGetIdToken(): () => string {
  const getAuthState = useContext(AuthStateContext);

  if (!getAuthState) {
    throw new Error('useGetIdToken must be used within AuthStateProvider');
  }

  return () => getAuthState().idToken || '';
}
