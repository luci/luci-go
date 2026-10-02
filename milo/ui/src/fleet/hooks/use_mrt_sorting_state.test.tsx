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

import { renderHook, act } from '@testing-library/react';
import { MRT_ColumnDef } from 'material-react-table';
import React from 'react';
import { MemoryRouter } from 'react-router';

import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import { useMrtSortingState } from './use_mrt_sorting_state';

describe('useMrtSortingState', () => {
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter
      initialEntries={['/test?order_by=hostname%20desc%2C%20status%20asc']}
    >
      <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
    </MemoryRouter>
  );

  it('parses multi-column orderBy parameters correctly', () => {
    const columns: MRT_ColumnDef<{ hostname: string; status: string }>[] = [
      { accessorKey: 'hostname', header: 'Hostname' },
      { accessorKey: 'status', header: 'Status' },
    ];

    const { result } = renderHook(() => useMrtSortingState(columns), {
      wrapper,
    });
    const [sorting] = result.current;

    expect(sorting).toEqual([
      { id: 'hostname', desc: true },
      { id: 'status', desc: false },
    ]);
  });

  it('resolves orderByField mapping when defined on columns', () => {
    const columns = [
      { id: 'model_col', orderByField: 'model' },
      { id: 'pool_col', orderByField: 'pool' },
    ];

    const customWrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/test?order_by=model%20desc%2C%20pool%20asc']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useMrtSortingState(columns), {
      wrapper: customWrapper,
    });
    const [sorting] = result.current;

    expect(sorting).toEqual([
      { id: 'model_col', desc: true },
      { id: 'pool_col', desc: false },
    ]);
  });

  it('resolves columns with accessorKey AND custom orderByField', () => {
    const columns = [
      { accessorKey: 'board', header: 'Board', orderByField: 'labels.board' },
    ];

    const customWrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/test?order_by=labels.board%20desc']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(
      () =>
        useMrtSortingState(
          columns as Array<{
            id?: string;
            orderByField?: string;
            accessorKey?: string;
          }>,
        ),
      {
        wrapper: customWrapper,
      },
    );
    const [sorting] = result.current;

    expect(sorting).toEqual([{ id: 'board', desc: true }]);
  });

  it('updates orderByParam when onSortingChange is invoked with custom orderByField', () => {
    const columns = [
      { accessorKey: 'board', header: 'Board', orderByField: 'labels.board' },
      { accessorKey: 'status', header: 'Status' },
    ];

    const customWrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/test']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(
      () =>
        useMrtSortingState(
          columns as Array<{
            id?: string;
            orderByField?: string;
            accessorKey?: string;
          }>,
        ),
      {
        wrapper: customWrapper,
      },
    );

    const [, onSortingChange] = result.current;

    act(() => {
      onSortingChange([
        { id: 'board', desc: true },
        { id: 'status', desc: false },
      ]);
    });

    const [, , orderByParam] = result.current;
    expect(orderByParam).toBe('labels.board desc, status');
  });

  it('falls back to orderByField id when column defines accessorFn without id or accessorKey', () => {
    const columns = [
      {
        header: 'Custom Metric',
        accessorFn: (row: { val: string }) => row.val,
        orderByField: 'labels.custom_metric',
      },
    ];

    const customWrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/test?order_by=labels.custom_metric%20desc']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(
      () =>
        useMrtSortingState(
          columns as unknown as MRT_ColumnDef<{ val: string }>[],
        ),
      {
        wrapper: customWrapper,
      },
    );
    const [sorting] = result.current;

    expect(sorting).toEqual([{ id: 'labels.custom_metric', desc: true }]);
  });
});
