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

import { useQuery } from '@tanstack/react-query';
import { createContext, useCallback, useContext, useEffect } from 'react';
import { useLatest } from 'react-use';

import { AuthState, msToExpire, queryAuthState } from '@/common/api/auth_state';
import { useStore } from '@/common/store';

export const AuthStateContext = createContext<(() => AuthState) | null>(null);

export interface AuthStateProviderProps {
  readonly initialValue: AuthState;
  readonly children: React.ReactNode;
}

export function AuthStateProvider({
  initialValue,
  children,
}: AuthStateProviderProps) {
  // Because the initial value could be an outdated value retrieved from cache,
  // do not wait until the initial value expires before sending out the first
  // query.
  const { data } = useQuery({
    queryKey: ['auth-state'],
    queryFn: () => queryAuthState(),
    initialData: initialValue,
    refetchInterval(prevData) {
      // This won't happen because `initialData` is provided.
      // The check is useful for type narrowing.
      if (!prevData) {
        // In case this does happen, wait at least 10s to avoid firing query
        // too rapidly (therefore taking up too much resources with no benefit).
        return 10000;
      }
      // Ensure there are at least 10s between updates so the backend
      // returning short-lived tokens (in case of a server bug) won't cause the
      // update action to fire rapidly (therefore taking up too much resources
      // with no benefit).
      //
      // Also expires the auth state 1 min earlier to the tokens won't expire
      // on the fly.
      return Math.max(msToExpire(prevData) - 60000, 10000);
    },
  });

  // Sync the auth state with the store so it can be used in Lit components.
  const store = useStore();
  useEffect(() => store.authState.setValue(data), [store, data]);

  const dataRef = useLatest(data);
  const getAuthState = useCallback(
    () => dataRef.current,
    // Establish a dependency on user identity so the provided getter is
    // refreshed whenever the identity changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [dataRef.current.identity]
  );

  return (
    <AuthStateContext.Provider value={getAuthState}>
      {children}
    </AuthStateContext.Provider>
  );
}

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
