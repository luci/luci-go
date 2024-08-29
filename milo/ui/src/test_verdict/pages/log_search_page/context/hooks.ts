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

import {
  LogGroupListDispatcherCtx,
  LogGroupListStateCtx,
  PaginationCtx,
  SearchFilterCtx,
} from './context';

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

export function useTestLogPagerCtx() {
  const ctx = useContext(PaginationCtx);
  if (ctx === undefined) {
    throw new Error(
      'useTestLogPagerCtx can only be used in a PaginationProvider',
    );
  }
  return ctx.testLogPagerCtx;
}

export function useInvocationLogPagerCtx() {
  const ctx = useContext(PaginationCtx);
  if (ctx === undefined) {
    throw new Error(
      'useInvocationLogPagerCtx can only be used in a PaginationProvider',
    );
  }
  return ctx.invocationLogPagerCtx;
}

export function useSearchFilter() {
  const ctx = useContext(SearchFilterCtx);
  if (ctx === undefined) {
    throw new Error(
      'useSearchFilter can only be used in a SearchFilterProvider',
    );
  }
  return ctx;
}
