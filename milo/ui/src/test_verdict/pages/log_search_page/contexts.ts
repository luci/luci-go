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

import { DateTime } from 'luxon';
import { useContext, createContext, Dispatch } from 'react';

import { PagerContext } from '@/common/components/params_pager';
import { Variant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

import { FormData } from './form_data';

export interface InvocationLogGroupIdentifier {
  readonly variantUnionHash: string;
  readonly variantUnion?: Variant;
  readonly artifactID: string;
}

export interface TestLogGroupIdentifier {
  readonly testID: string;
  readonly variantHash: string;
  readonly variant?: Variant;
  readonly artifactID: string;
}

export interface LogGroupListState {
  readonly testLogGroupIdentifier: TestLogGroupIdentifier | null;
  readonly invocationLogGroupIdentifier: InvocationLogGroupIdentifier | null;
}

interface ShowTestLogGroupListAction {
  readonly type: 'showTestLogGroupList';
  readonly logGroupIdentifer: TestLogGroupIdentifier;
}

interface ShowInvocationGroupListAction {
  readonly type: 'showInvocationLogGroupList';
  readonly logGroupIdentifer: InvocationLogGroupIdentifier;
}

interface DismissAction {
  readonly type: 'dismiss';
}

export type Action =
  | ShowTestLogGroupListAction
  | ShowInvocationGroupListAction
  | DismissAction;

export function reducer(
  _state: LogGroupListState,
  action: Action,
): LogGroupListState {
  switch (action.type) {
    case 'showTestLogGroupList':
      return {
        testLogGroupIdentifier: action.logGroupIdentifer,
        invocationLogGroupIdentifier: null,
      };
    case 'showInvocationLogGroupList':
      return {
        testLogGroupIdentifier: null,
        invocationLogGroupIdentifier: action.logGroupIdentifer,
      };
    case 'dismiss':
      return {
        testLogGroupIdentifier: null,
        invocationLogGroupIdentifier: null,
      };
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

export interface LogPaginationState {
  readonly testLogPagerCtx: PagerContext;
  readonly invocationLogPagerCtx: PagerContext;
}

export const PaginationCtx = createContext<LogPaginationState | null>(null);

export function useTestLogPagerCtx() {
  const ctx = useContext(PaginationCtx);
  if (!ctx) {
    throw new Error(
      'useTestLogPagerCtx can only be used in a PaginationProvider',
    );
  }
  return ctx.testLogPagerCtx;
}

export function useInvocationLogPagerCtx() {
  const ctx = useContext(PaginationCtx);
  if (!ctx) {
    throw new Error(
      'useInvocationLogPagerCtx can only be used in a PaginationProvider',
    );
  }
  return ctx.invocationLogPagerCtx;
}

export interface SearchFilter {
  readonly form: FormData;
  readonly startTime: DateTime;
  readonly endTime: DateTime;
}

/**
 * SearchFilterCtx holds all fields required for a log search.
 * Use null to represent the filters are not set, and search should not be initiate.
 * Use undefine to represent no matching provider in the tree above.
 */
export const SearchFilterCtx = createContext<SearchFilter | null | undefined>(
  undefined,
);

export function useSearchFilter() {
  const ctx = useContext(SearchFilterCtx);
  if (ctx === undefined) {
    throw new Error(
      'useSearchFilter can only be used in a SearchFilterProvider',
    );
  }
  return ctx;
}
