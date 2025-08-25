// Copyright 2025 The LUCI Authors.
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

import { GridColDef } from '@mui/x-data-grid';
import { act, renderHook } from '@testing-library/react';

import { fakeLocalStorage } from '@/fleet/testing_tools/mocks/fake_local_storage';
import { fakeUseSyncedSearchParams } from '@/fleet/testing_tools/mocks/fake_search_params';

import {
  ColumnManagementConfig,
  useColumnManagement,
} from './use_column_management';

const ALL_COLUMNS: readonly GridColDef[] = [
  { field: 'name', headerName: 'Name' },
  { field: 'os', headerName: 'OS' },
  { field: 'pool', headerName: 'Pool' },
  { field: 'cpu', headerName: 'CPU' },
  { field: 'memory', headerName: 'Memory' },
];
const DEFAULT_COLUMNS = ['name', 'os'];
const LOCAL_STORAGE_KEY = 'test-columns-key';

describe('useColumnManagement', () => {
  const config: ColumnManagementConfig = {
    allColumns: ALL_COLUMNS,
    highlightedColumnIds: [],
    defaultColumns: DEFAULT_COLUMNS,
    localStorageKey: LOCAL_STORAGE_KEY,
  };

  beforeEach(() => {
    jest.clearAllMocks();

    fakeUseSyncedSearchParams();
    fakeLocalStorage();
  });

  it('default columns are not persisted in local storage if not modified by the user', () => {
    renderHook(useColumnManagement, {
      initialProps: { ...config, defaultColumns: ['cpu', 'pool'] },
    });

    const { result } = renderHook(useColumnManagement, {
      initialProps: config,
    });
    expect(result.current.columnVisibilityModel).toEqual({
      name: true, // current default columns are used
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });
  });

  it('should initialize with default columns and dynamic order', () => {
    const { result: result1 } = renderHook(useColumnManagement, {
      initialProps: config, // saves it to local storage in a separate render
    });

    act(() => {
      result1.current.onColumnVisibilityModelChange({
        name: false,
        os: false,
        pool: true,
        cpu: true,
        memory: false,
      });
    });

    // new render will dehydrate from local storage
    const { result } = renderHook(useColumnManagement, {
      initialProps: config,
    });
    expect(result.current.columnVisibilityModel).toEqual({
      name: false,
      os: false,
      pool: true,
      cpu: true,
      memory: false,
    });
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'cpu',
      'pool',
      'memory',
      'name',
      'os',
    ]);
  });

  it('should use user-selected columns if they exist', () => {
    const { result } = renderHook(useColumnManagement, {
      initialProps: config,
    });
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });
    // Order should match the user's preference.
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'name',
      'os',
      'cpu',
      'memory',
      'pool',
    ]);
  });

  it('should make temporary column visible', () => {
    const { result } = renderHook(useColumnManagement, {
      initialProps: { ...config, highlightedColumnIds: ['pool'] },
    });
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: true, // Temporarily visible.
      cpu: false,
      memory: false,
    });

    const tempColumn = result.current.columns.find((c) => c.field === 'pool');
    expect(tempColumn).toBeDefined();
    expect(tempColumn?.hideable).toBe(false);
    expect(tempColumn?.headerClassName).toBe(
      'column-highlight temporary-column-highlight',
    );
    expect(tempColumn?.cellClassName).toBe(
      'column-highlight temporary-column-highlight',
    );
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'name',
      'os',
      'pool',
      'cpu',
      'memory',
    ]);
  });

  it('should highlight highlighted column', () => {
    const { result } = renderHook(useColumnManagement, {
      initialProps: { ...config, highlightedColumnIds: ['os'] },
    });
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });

    const highlightedColumn = result.current.columns.find(
      (c) => c.field === 'os',
    );
    expect(highlightedColumn).toBeDefined();
    expect(highlightedColumn?.hideable).toBe(undefined);
    expect(highlightedColumn?.headerClassName).toBe('column-highlight');
    expect(highlightedColumn?.cellClassName).toBe('column-highlight');
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'name',
      'os',
      'cpu',
      'memory',
      'pool',
    ]);
  });

  it('should update user-visible columns when visibility model changes', () => {
    const { result, rerender } = renderHook(useColumnManagement, {
      initialProps: config,
    });
    act(() => {
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: false, // User hides 'os'.
        pool: false,
        cpu: true, // User shows 'cpu'.
        memory: false,
      });
    });

    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: false,
      pool: false,
      cpu: true,
      memory: false,
    });

    rerender(config);

    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: false,
      pool: false,
      cpu: true,
      memory: false,
    });
  });

  it('should prevent temporary columns from being saved to user preferences', () => {
    // first render, will hydrate local storage
    const { result: firstRender } = renderHook(useColumnManagement, {
      initialProps: { ...config, highlightedColumnIds: ['pool'] },
    });

    expect(firstRender.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: true,
      cpu: false,
      memory: false,
    });

    // second render, will dehydrate from local storage
    const { result } = renderHook(useColumnManagement, {
      initialProps: { ...config },
    });

    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });
  });

  it('should prevent temporary columns from being hidden via the visibility model', () => {
    const { result } = renderHook(useColumnManagement, {
      initialProps: { ...config, highlightedColumnIds: ['pool'] },
    });
    act(() => {
      // User tries to hide the temporary 'pool' column and a permanent 'os' column.
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: true,
        pool: false,
        cpu: false,
        memory: false,
      });
    });

    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: true,
      cpu: false,
      memory: false,
    });
  });

  it('should reset columns to default when resetDefaultColumns is called', () => {
    const { result } = renderHook(useColumnManagement, {
      initialProps: config,
    });

    act(() =>
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: true,
        pool: true, // user switched to 'true'
        cpu: false,
        memory: false,
      }),
    );

    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: true,
      cpu: false,
      memory: false,
    });

    act(() => result.current.resetDefaultColumns());
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });
  });

  it('should preserve original column order when preserveOrder is true', () => {
    const { result } = renderHook(() =>
      useColumnManagement({ ...config, preserveOrder: true }),
    );

    // The final order should match the order in ALL_COLUMNS, not the user's preference.
    // 'name' comes before 'cpu' in ALL_COLUMNS.
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'name',
      'os',
      'pool',
      'cpu',
      'memory',
    ]);
  });
});
