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

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { useColumnManagement } from './use_column_management';
import { useParamsAndLocalStorage } from './use_params_and_local_storage';

// Mock dependencies from other modules.
jest.mock('@/generic_libs/hooks/synced_search_params');
jest.mock('./use_params_and_local_storage');
jest.mock('./columns', () => ({
  orderColumns: (cols: { field: string }[], order: string[]) => {
    const colMap = new Map(cols.map((c) => [c.field, c]));
    return order.map((id) => colMap.get(id)).filter(Boolean);
  },
}));

const mockUseSyncedSearchParams = useSyncedSearchParams as jest.Mock;
const mockUseParamsAndLocalStorage = useParamsAndLocalStorage as jest.Mock;

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
  let mockSetUserVisibleColumns: jest.Mock;

  const config = {
    allColumns: ALL_COLUMNS,
    defaultColumns: DEFAULT_COLUMNS,
    localStorageKey: LOCAL_STORAGE_KEY,
  };

  beforeEach(() => {
    jest.clearAllMocks();
    mockSetUserVisibleColumns = jest.fn();

    // Default mock setup for a clean state.
    mockUseSyncedSearchParams.mockReturnValue([new URLSearchParams()]);
    mockUseParamsAndLocalStorage.mockReturnValue([
      [...DEFAULT_COLUMNS],
      mockSetUserVisibleColumns,
    ]);
  });

  it('should initialize with default columns and dynamic order', () => {
    const { result } = renderHook(() => useColumnManagement(config));
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: false,
      cpu: false,
      memory: false,
    });
    expect(result.current.columns.map((c) => c.field)).toEqual(['name', 'os']);
  });

  it('should use user-selected columns if they exist', () => {
    mockUseParamsAndLocalStorage.mockReturnValue([
      ['cpu', 'name'],
      mockSetUserVisibleColumns,
    ]);

    const { result } = renderHook(() => useColumnManagement(config));
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: false,
      pool: false,
      cpu: true,
      memory: false,
    });
    // Order should match the user's preference.
    expect(result.current.columns.map((c) => c.field)).toEqual(['cpu', 'name']);
  });

  it('should make a hidden column temporarily visible if it has an active filter', () => {
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams('filters=labels.pool = "p1"'),
    ]);

    const { result } = renderHook(() => useColumnManagement(config));
    expect(result.current.columnVisibilityModel).toEqual({
      name: true,
      os: true,
      pool: true, // Temporarily visible.
      cpu: false,
      memory: false,
    });

    const poolColumn = result.current.columns.find((c) => c.field === 'pool');
    expect(poolColumn).toBeDefined();
    expect(poolColumn?.hideable).toBe(false);
    expect(poolColumn?.headerClassName).toBe('temp-visible-column');
    expect(poolColumn?.cellClassName).toBe('temp-visible-column');
    expect(result.current.columns.map((c) => c.field)).toEqual([
      'name',
      'os',
      'pool',
    ]);
  });

  it('should update user-visible columns when visibility model changes', () => {
    const { result } = renderHook(() => useColumnManagement(config));
    act(() => {
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: false, // User hides 'os'.
        pool: false,
        cpu: true, // User shows 'cpu'.
        memory: false,
      });
    });
    expect(mockSetUserVisibleColumns).toHaveBeenCalledWith(['name', 'cpu']);
  });

  it('should prevent temporary columns from being saved to user preferences', () => {
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams('filters=labels.pool = "p1"'), // 'pool' is temporary.
    ]);
    const { result } = renderHook(() => useColumnManagement(config));
    act(() => {
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: true,
        pool: true, // This is temporary and should not be saved.
        cpu: true, // User adds 'cpu'.
        memory: false,
      });
    });

    // 'pool' should be filtered out from the saved preferences.
    expect(mockSetUserVisibleColumns).toHaveBeenCalledWith([
      'name',
      'os',
      'cpu',
    ]);
  });

  it('should prevent temporary columns from being hidden via the visibility model', () => {
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams('filters=labels.pool = "p1"'), // 'pool' is temporary.
    ]);
    const { result } = renderHook(() => useColumnManagement(config));
    act(() => {
      // User tries to hide the temporary 'pool' column and a permanent 'os' column.
      result.current.onColumnVisibilityModelChange({
        name: true,
        os: false,
        pool: false,
        cpu: false,
        memory: false,
      });
    });

    // The change to 'os' is saved, but the attempt to hide 'pool' is ignored.
    expect(mockSetUserVisibleColumns).toHaveBeenCalledWith(['name']);
  });

  it('should reset columns to default when resetDefaultColumns is called', () => {
    mockUseParamsAndLocalStorage.mockReturnValue([
      ['name', 'cpu', 'memory'], // Start with custom columns.
      mockSetUserVisibleColumns,
    ]);
    const { result } = renderHook(() => useColumnManagement(config));
    act(() => result.current.resetDefaultColumns());
    expect(mockSetUserVisibleColumns).toHaveBeenCalledWith([
      ...DEFAULT_COLUMNS,
    ]);
  });

  it('should preserve original column order when preserveOrder is true', () => {
    // User has selected columns in a different order than the original definition.
    mockUseParamsAndLocalStorage.mockReturnValue([
      ['cpu', 'name'],
      mockSetUserVisibleColumns,
    ]);

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
