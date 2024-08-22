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

import { useContext } from 'react';

import {
  CommitContext,
  DefaultExpandedContext,
  ExpandedContext,
  ExpandStateStoreContext,
  RepoContext,
  SetDefaultExpandedContext,
  SetExpandedContext,
  TableRowIndexContext,
  TableRowPropsContext,
  TableSxContext,
} from './context';

export function useRepoUrl() {
  const ctx = useContext(RepoContext);

  if (ctx === undefined) {
    throw new Error('useRepoUrl can only be used in a CommitTable');
  }

  return ctx;
}

export function useTableSx() {
  return useContext(TableSxContext);
}

export function useTableRowProps() {
  return useContext(TableRowPropsContext);
}

export function useTableRowIndex() {
  return useContext(TableRowIndexContext);
}

export function useDefaultExpanded() {
  const ctx = useContext(DefaultExpandedContext);

  if (ctx === undefined) {
    throw new Error('useDefaultExpanded can only be used in a CommitTable');
  }

  return ctx;
}

export function useSetDefaultExpanded() {
  const ctx = useContext(SetDefaultExpandedContext);

  if (ctx === undefined) {
    throw new Error('useSetDefaultExpanded can only be used in a CommitTable');
  }

  return ctx;
}

export function useExpandStateStore() {
  return useContext(ExpandStateStoreContext);
}

export function useExpanded() {
  const ctx = useContext(ExpandedContext);

  if (ctx === undefined) {
    throw new Error('useExpanded can only be used in a CommitTableRow');
  }

  return ctx;
}

export function useSetExpanded() {
  const ctx = useContext(SetExpandedContext);

  if (ctx === undefined) {
    throw new Error('useSetExpanded can only be used in a CommitTable');
  }

  return ctx;
}

/**
 * Get the commit to be rendered in this row.
 *
 * `null` is returned when the commit it not loaded yet.
 */
export function useCommit() {
  const ctx = useContext(CommitContext);

  if (ctx === undefined) {
    throw new Error('useCommit can only be used in a CommitTableRow');
  }

  return ctx;
}
