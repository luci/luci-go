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

import { useMRTColumnManagement } from './use_mrt_column_management';
import { useParamsAndLocalStorage } from './use_params_and_local_storage';

// Mock dependencies
jest.mock('./use_params_and_local_storage', () => ({
  useParamsAndLocalStorage: jest.fn(),
}));

describe('useMRTColumnManagement', () => {
  // Use simple objects that pretend to be MRT_ColumnDef
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const mockColumns: MRT_ColumnDef<any>[] = [
    { id: 'col1', header: 'Column 1' },
    { id: 'col2', header: 'Column 2' },
    { accessorKey: 'col3', header: 'Column 3' }, // Test accessorKey fallback
  ];

  beforeEach(() => {
    (useParamsAndLocalStorage as jest.Mock).mockReset();
  });

  it('initializes visibility correctly', () => {
    // Mock return: [currentValue, setter]
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col1', 'col3'],
      jest.fn(),
    ]);

    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
      }),
    );

    expect(result.current.columnVisibility).toEqual({
      col1: true,
      col2: false,
      col3: true,
    });
    expect(result.current.visibleColumnIds).toEqual(['col1', 'col3']);
  });

  it('toggles column visibility off', () => {
    const setVisibleIds = jest.fn();
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col1', 'col2'],
      setVisibleIds,
    ]);

    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
      }),
    );

    act(() => {
      result.current.onToggleColumn('col1');
    });

    expect(setVisibleIds).toHaveBeenCalledWith(['col2']);
  });

  it('toggles column visibility on', () => {
    const setVisibleIds = jest.fn();
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col1'],
      setVisibleIds,
    ]);

    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
      }),
    );

    act(() => {
      result.current.onToggleColumn('col2');
    });

    expect(setVisibleIds).toHaveBeenCalledWith(['col1', 'col2']);
  });

  it('setColumnVisibility handles function updater', () => {
    const setVisibleIds = jest.fn();
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col1'],
      setVisibleIds,
    ]);

    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
      }),
    );

    act(() => {
      // simulate MRT passing a function to update visibility
      result.current.setColumnVisibility((old) => ({
        ...old,
        col2: true,
      }));
    });

    // Old visibility was {col1: true, col2: false, col3: false}
    // New visibility should be {col1: true, col2: true, col3: false}
    // Resulting IDs: ['col1', 'col2']
    expect(setVisibleIds).toHaveBeenCalledWith(
      expect.arrayContaining(['col1', 'col2']),
    );
  });

  it('resetDefaultColumns resets to default', () => {
    const setVisibleIds = jest.fn();
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col2'],
      setVisibleIds,
    ]);

    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
      }),
    );

    act(() => {
      result.current.resetDefaultColumns();
    });

    expect(setVisibleIds).toHaveBeenCalledWith(['col1']);
  });

  it('temporary columns remain visible when highlighted', () => {
    // Return an empty visible list to mock all columns being hidden by default
    const setVisibleIds = jest.fn();
    (useParamsAndLocalStorage as jest.Mock).mockReturnValue([
      ['col1'],
      setVisibleIds,
    ]);

    // Pass in col2 and col3 as currently highlighted/filtered
    const { result } = renderHook(() =>
      useMRTColumnManagement({
        columns: mockColumns,
        defaultColumnIds: ['col1'],
        localStorageKey: 'test-key',
        highlightedColumnIds: ['col2', 'col3'],
      }),
    );

    // Visibility state should contain the highlighted/temp ones dynamically added back in
    expect(result.current.columnVisibility).toEqual({
      col1: true,
      col2: true,
      col3: true,
    });

    // Original list remains unaffected
    expect(result.current.visibleColumnIds).toEqual(['col1']);

    // They should inherit isTemporary properties in the mapped columns
    const isCol2Temp = (
      result.current.columns.find((c) => c.id === 'col2')?.meta as {
        isTemporary?: boolean;
      }
    )?.isTemporary;
    expect(isCol2Temp).toBe(true);
  });
});
