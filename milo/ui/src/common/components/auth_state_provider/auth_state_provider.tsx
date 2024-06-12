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
import { useEffect, useMemo, useRef } from 'react';

import {
  AuthState,
  msToExpire,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';
import { useStore } from '@/common/store';
import { deferred } from '@/generic_libs/tools/utils';

import { AUTH_STATE_QUERY_KEY } from './constants';
import { AuthStateContext } from './context';

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

  const authStateRef = useRef(authState);
  authStateRef.current = authState;
  const ctxValue = useMemo(() => {
    // Establish a dependency on user identity so the provided getters are
    // refreshed whenever the identity changes.
    authState.identity;

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
    async function getValidAuthState() {
      if (msToExpire(authStateRef.current) >= TOKEN_BUFFER_DURATION_MS) {
        return authStateRef.current;
      }
      return nextValidAuthStateHandlesRef.current[0];
    }
    return {
      getAuthState: () => authStateRef.current,
      getAccessToken: async () => (await getValidAuthState()).accessToken || '',
      getIdToken: async () => (await getValidAuthState()).idToken || '',
    };
  }, [authState.identity]);

  return (
    <AuthStateContext.Provider value={ctxValue}>
      {children}
    </AuthStateContext.Provider>
  );
}
