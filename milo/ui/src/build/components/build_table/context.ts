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

import { createContext, useContext } from 'react';

import { Build } from '@/common/services/buildbucket';
import { ExpandableEntriesStateInstance } from '@/common/store/expandable_entries_state';

const TableStateContext = createContext<ExpandableEntriesStateInstance | null>(
  null
);

export const TableStateProvider = TableStateContext.Provider;

export function useTableState() {
  const state = useContext(TableStateContext);

  if (!state) {
    throw new Error('useTableState must be used within TableStateProvider');
  }

  return state;
}

const RowStateContext = createContext<Build | null>(null);

export const RowStateProvider = RowStateContext.Provider;

export function useRowState() {
  const state = useContext(RowStateContext);

  if (!state) {
    throw new Error('useRowState must be used within RowStateProvider');
  }

  return state;
}
