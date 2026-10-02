// Copyright 2026 The LUCI Authors.
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

import { renderHook } from '@testing-library/react';
import { MRT_ColumnDef } from 'material-react-table';
import React from 'react';
import { MemoryRouter } from 'react-router';

import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import { useMrtSorting } from './use_mrt_sorting';

interface TestRow {
  id: string;
  name?: string | null;
  nested: { score: number };
}

describe('useMrtSorting', () => {
  const rows: TestRow[] = [
    { id: '1', name: 'beta', nested: { score: 10 } },
    { id: '2', name: 'alpha', nested: { score: 30 } },
    { id: '3', name: 'alpha', nested: { score: 20 } },
    { id: '4', name: null, nested: { score: 5 } },
  ];

  const columns: MRT_ColumnDef<TestRow>[] = [
    { id: 'name', accessorKey: 'name', header: 'Name' },
    {
      id: 'score',
      header: 'Score',
      accessorFn: (row) => row.nested.score,
      sortingFn: (rowA, rowB, columnId) =>
        Number(rowA.getValue(columnId)) - Number(rowB.getValue(columnId)),
    },
  ];

  it('sorts using accessorFn and custom sortingFn', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/test?order_by=score%20desc']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useMrtSorting(rows, columns), {
      wrapper,
    });

    expect(result.current.sortedRows.map((r) => r.id)).toEqual([
      '2',
      '3',
      '1',
      '4',
    ]);
  });

  it('supports multi-column tie-breaking and handles null/undefined values', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/test?order_by=name%20asc%2C%20score%20asc']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useMrtSorting(rows, columns), {
      wrapper,
    });

    expect(result.current.sortedRows.map((r) => r.id)).toEqual([
      '4',
      '3',
      '2',
      '1',
    ]);
  });
});
