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

import {
  createContext,
  Dispatch,
  ReactNode,
  useContext,
  useReducer,
} from 'react';

import { Action, BlamelistState, reducer } from './reducer';

const BlamelistDispatcherCtx = createContext<Dispatch<Action> | undefined>(
  undefined,
);
const BlamelistStateCtx = createContext<BlamelistState | undefined>(undefined);

export interface BlamelistStateProviderProps {
  readonly children: ReactNode;
}

export function BlamelistStateProvider({
  children,
}: BlamelistStateProviderProps) {
  const [state, dispatch] = useReducer(reducer, {
    testVariantBranch: null,
    focusCommitPosition: null,
  });

  return (
    <BlamelistDispatcherCtx.Provider value={dispatch}>
      <BlamelistStateCtx.Provider value={state}>
        {children}
      </BlamelistStateCtx.Provider>
    </BlamelistDispatcherCtx.Provider>
  );
}

export function useBlamelistDispatch() {
  const ctx = useContext(BlamelistDispatcherCtx);
  if (ctx === undefined) {
    throw new Error(
      'useBlamelistDispatch can only be used within BlamelistStateProvider',
    );
  }
  return ctx;
}

export function useBlamelistState() {
  const ctx = useContext(BlamelistStateCtx);
  if (ctx === undefined) {
    throw new Error(
      'useBlamelistState can only be used within BlamelistStateProvider',
    );
  }
  return ctx;
}
