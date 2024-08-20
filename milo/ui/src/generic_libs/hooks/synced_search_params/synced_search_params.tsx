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

import { ReactNode, createContext, useCallback, useMemo } from 'react';
import {
  NavigateOptions,
  // `useSyncedSearchParams` replaces `useSearchParams` but its own implementation
  // depends on `useSearchParams`.
  // eslint-disable-next-line no-restricted-imports
  useSearchParams,
} from 'react-router-dom';
import { useLatest } from 'react-use';

type SetURLSearchParams = (
  action: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams),
  navigateOpts?: NavigateOptions,
) => void;

export const SyncedSearchParamsContext = createContext<
  readonly [URLSearchParams, SetURLSearchParams] | null
>(null);

export interface ContextProviderProps {
  readonly children: ReactNode;
}

export function SyncedSearchParamsProvider({ children }: ContextProviderProps) {
  const [state, setState] = useSearchParams();

  const depsRef = useLatest({ state, setState });

  const setSearchParams = useCallback<SetURLSearchParams>(
    (action, navigateOpts?) => {
      const { state, setState } = depsRef.current;
      const newState = typeof action === 'function' ? action(state) : action;
      // This ensures calling the dispatch multiple times will not cause the
      // result from the previous actions to be ignored when `action` is an
      // updater.
      depsRef.current.state = newState;
      setState(newState, navigateOpts);
    },
    [depsRef],
  );

  const ctxValue = useMemo(
    () => [state, setSearchParams] as const,
    [state, setSearchParams],
  );

  return (
    <SyncedSearchParamsContext.Provider value={ctxValue}>
      {children}
    </SyncedSearchParamsContext.Provider>
  );
}
