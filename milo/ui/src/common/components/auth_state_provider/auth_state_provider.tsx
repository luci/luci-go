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
import { createContext, useContext, useEffect, useMemo, useRef } from 'react';
import { useLatest } from 'react-use';

import {
  AuthState,
  msToExpire,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';
import { useStore } from '@/common/store';
import { deferred } from '@/generic_libs/tools/utils';

/**
 * Minimum internal between auth state queries in milliseconds.
 *
 * Ensure there are at least some time between updates so the backend returning
 * short-lived tokens (in case of a server bug) won't cause the query to be
 * fired rapidly (therefore taking up too much resources with no benefit).
 */
const MIN_QUERY_INTERVAL_MS = 10000;

/**
 * Expires the auth state tokens a bit early to ensure that the tokens won't
 * expire on the fly.
 */
const TOKEN_BUFFER_DURATION_MS = 10000;

interface AuthStateContextValue {
  readonly getAuthState: () => AuthState;
  readonly getValidAuthState: () => Promise<AuthState>;
}

export const AUTH_STATE_QUERY_KEY = Object.freeze(['auth-state']);

export const AuthStateContext = createContext<AuthStateContextValue | null>(
  null,
);

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
  const { data, isPlaceholderData } = useQuery({
    queryKey: AUTH_STATE_QUERY_KEY,
    queryFn: () => queryAuthState(),
    placeholderData: initialValue,
    refetchInterval(prevData) {
      if (!prevData) {
        return MIN_QUERY_INTERVAL_MS;
      }
      // Expires the auth state 1 min earlier to the tokens won't expire
      // on the fly.
      return Math.max(msToExpire(prevData) - 60000, MIN_QUERY_INTERVAL_MS);
    },
  });
  // Placeholder data is provided. Cannot be null.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  const authState = data!;

  // Populate the auth state cache so it can be used by other browser tabs on
  // start up.
  useEffect(() => {
    if (isPlaceholderData) {
      return;
    }
    setAuthStateCache(authState);
  }, [isPlaceholderData, authState]);

  // Sync the auth state with the store so it can be used in Lit components.
  const store = useStore();
  useEffect(() => store.authState.setValue(authState), [store, authState]);

  // A tuple where
  // 1. the first element is a promise that resolves when the next valid auth
  //    state OF THE SAME IDENTITY is available, and
  // 2. the second element is a function that can mark the first element as
  //    resolved.
  const nextValidAuthStateHandlesRef = useRef(deferred<AuthState>());
  useEffect(() => {
    // Reset the handles since we will never get a valid auth state of the
    // previous user.
    nextValidAuthStateHandlesRef.current = deferred();
  }, [authState.identity]);
  useEffect(() => {
    if (msToExpire(authState) < TOKEN_BUFFER_DURATION_MS) {
      return;
    }
    const [_, resolveNextAuthState] = nextValidAuthStateHandlesRef.current;
    resolveNextAuthState(authState);
    // Since a new auth state has been received, create new handles for the
    // next "next valid auth state".
    nextValidAuthStateHandlesRef.current = deferred();
  }, [authState]);

  const authStateRef = useLatest(authState);
  const ctxValue = useMemo(
    () => ({
      getAuthState: () => authStateRef.current,

      // Build a function that returns the next valid auth state with
      // `nextValidAuthStateHandlesRef`.
      // Simply using `obtainAuthState` from `@/common/api/auth_state` is not
      // ideal because
      // 1. on refocus, if the cached value has expired, multiple queries will
      //    be sent at the same time before any of them get the response to
      //    populate the cache, causing unnecessary network requests, and
      // 2. `obtainAuthState` could return tokens that belong to a different
      //    user (to the user identity indicated by the cached auth state),
      //    which may cause problems if the query is cached with the user
      //    identity at call time as part of the cache key.
      getValidAuthState: async () => {
        if (msToExpire(authStateRef.current) >= TOKEN_BUFFER_DURATION_MS) {
          return authStateRef.current;
        }
        return nextValidAuthStateHandlesRef.current[0];
      },
    }),
    // Establish a dependency on user identity so the provided getter is
    // refreshed whenever the identity changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [authState.identity],
  );

  return (
    <AuthStateContext.Provider value={ctxValue}>
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
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useAuthState must be used within AuthStateProvider');
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
 */
export function useGetAccessToken(): () => Promise<string> {
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useGetAccessToken must be used within AuthStateProvider');
  }

  return async () => {
    return (await value.getValidAuthState()).accessToken || '';
  };
}

/**
 * Returns a function that resolves the latest non-expired ID token of the
 * current user when invoked.
 *
 * Context update happens WHEN AND ONLY WHEN the user identity changes (which
 * can happen if the user logged into a different account via a browser tab
 * between auth state refreshes).
 */
export function useGetIdToken(): () => Promise<string> {
  const value = useContext(AuthStateContext);

  if (!value) {
    throw new Error('useGetIdToken must be used within AuthStateProvider');
  }

  return async () => {
    return (await value.getValidAuthState()).idToken || '';
  };
}
