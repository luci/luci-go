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

import { useContext, createContext, Dispatch } from 'react';

import { Variant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

export interface LogGroupIdentifier {
  readonly testID: string;
  readonly variantHash: string;
  readonly variant?: Variant;
  readonly artifactID: string;
}

export interface LogGroupListState {
  readonly logGroupIdentifer: LogGroupIdentifier | null;
}

interface ShowLogGroupListAction {
  readonly type: 'showLogGroupList';
  readonly logGroupIdentifer: LogGroupIdentifier | null;
}

interface DismissAction {
  readonly type: 'dismiss';
}

type Action = ShowLogGroupListAction | DismissAction;

export function reducer(
  _state: LogGroupListState,
  action: Action,
): LogGroupListState {
  switch (action.type) {
    case 'showLogGroupList':
      return {
        logGroupIdentifer: action.logGroupIdentifer,
      };
    case 'dismiss':
      return { logGroupIdentifer: null };
  }
}

export const LogGroupListDispatcherCtx = createContext<
  Dispatch<Action> | undefined
>(undefined);

export const LogGroupListStateCtx = createContext<
  LogGroupListState | undefined
>(undefined);

export function useLogGroupListDispatch() {
  const ctx = useContext(LogGroupListDispatcherCtx);
  if (ctx === undefined) {
    throw new Error(
      'useLogGroupListDispatch can only be used in a LogGroupListStateProvider',
    );
  }
  return ctx;
}

export function useLogGroupListState() {
  const ctx = useContext(LogGroupListStateCtx);
  if (ctx === undefined) {
    throw new Error(
      'useLogGroupListState can only be used in a LogGroupListStateProvider',
    );
  }
  return ctx;
}
