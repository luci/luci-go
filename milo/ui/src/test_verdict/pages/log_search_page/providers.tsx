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

import { ReactNode, useReducer } from 'react';

import {
  reducer,
  LogGroupListDispatcherCtx,
  LogGroupListStateCtx,
} from './contexts';

export interface LogGroupListStateProviderProps {
  readonly children: ReactNode;
}

export function LogGroupListStateProvider({
  children,
}: LogGroupListStateProviderProps) {
  const [state, dispatch] = useReducer(reducer, {
    logGroupIdentifer: null,
  });

  return (
    <LogGroupListDispatcherCtx.Provider value={dispatch}>
      <LogGroupListStateCtx.Provider value={state}>
        {children}
      </LogGroupListStateCtx.Provider>
    </LogGroupListDispatcherCtx.Provider>
  );
}
