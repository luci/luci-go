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

import { act, renderHook } from '@testing-library/react';
import React from 'react';

import { PagerContext } from '@/common/components/params_pager/context';
import { OptionCategory } from '@/fleet/types/option';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useFleetMRTState, FleetColumnDefExt } from './use_fleet_mrt_state';

describe('useFleetMRTState', () => {
  const localStorageKey = 'test-table-columns';
  const defaultColumnIds = ['col1', 'col2'];
  const visibleColumns: FleetColumnDefExt[] = [
    { id: 'col1', accessorKey: 'col1' },
    { id: 'col2', accessorKey: 'col2' },
  ];

  const mockSetSearchParams = jest.fn();
  const mockPagerCtx = {} as PagerContext;
  const mockFilterValues = {};
  const mockFilterOptionsConfig: OptionCategory[] = [];

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <FakeContextProvider
      mountedPath="/test/:platform"
      routerOptions={{
        initialEntries: ['/test/chromeos'],
      }}
    >
      {children}
    </FakeContextProvider>
  );

  beforeEach(() => {
    localStorage.clear();
    jest.clearAllMocks();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('should load default empty column sizing from localStorage if none is stored', () => {
    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    expect(result.current.columnSizing).toEqual({});
  });

  it('should load stored column sizing from localStorage if present', () => {
    const storedSizes = { col1: 150, col2: 300 };
    localStorage.setItem(
      `${localStorageKey}-sizes`,
      JSON.stringify(storedSizes),
    );

    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    expect(result.current.columnSizing).toEqual(storedSizes);
  });

  it('should update column sizing and save to localStorage on change', async () => {
    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    const newSizes = { col1: 200 };

    act(() => {
      result.current.onColumnSizingChange(newSizes);
    });

    expect(result.current.columnSizing).toEqual(newSizes);

    // Wait for the 500ms debounce write to occur
    await act(async () => {
      await jest.advanceTimersByTimeAsync(500);
    });

    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toEqual(
      JSON.stringify(newSizes),
    );
  });

  it('should handle functional updater for columnSizing', async () => {
    const initialSizes = { col1: 150 };
    localStorage.setItem(
      `${localStorageKey}-sizes`,
      JSON.stringify(initialSizes),
    );

    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    act(() => {
      result.current.onColumnSizingChange((prev) => ({
        ...prev,
        col2: 250,
      }));
    });

    const expectedSizes = { col1: 150, col2: 250 };
    expect(result.current.columnSizing).toEqual(expectedSizes);

    // Wait for the 500ms debounce write to occur
    await act(async () => {
      await jest.advanceTimersByTimeAsync(500);
    });

    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toEqual(
      JSON.stringify(expectedSizes),
    );
  });

  it('should sync column sizing when localStorageKey changes', () => {
    const key1 = 'key-1';
    const key2 = 'key-2';
    const sizes1 = { col1: 100 };
    const sizes2 = { col2: 200 };

    localStorage.setItem(`${key1}-sizes`, JSON.stringify(sizes1));
    localStorage.setItem(`${key2}-sizes`, JSON.stringify(sizes2));

    let currentKey = key1;

    const { result, rerender } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey: currentKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    expect(result.current.columnSizing).toEqual(sizes1);

    currentKey = key2;
    rerender();

    expect(result.current.columnSizing).toEqual(sizes2);
  });

  it('should not overwrite the new key storage with the old key value during transition', async () => {
    const key1 = 'key-1';
    const key2 = 'key-2';
    const sizes1 = { col1: 100 };
    const sizes2 = { col2: 200 };

    localStorage.setItem(`${key1}-sizes`, JSON.stringify(sizes1));
    localStorage.setItem(`${key2}-sizes`, JSON.stringify(sizes2));

    let currentKey = key1;

    const { result, rerender } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey: currentKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    expect(result.current.columnSizing).toEqual(sizes1);

    // Transition to key2
    currentKey = key2;
    rerender();

    // Wait for any pending/cleared timers to complete
    await act(async () => {
      await jest.advanceTimersByTimeAsync(500);
    });

    // Verify that key2 is still sizes2 and has not been overwritten with sizes1
    expect(result.current.columnSizing).toEqual(sizes2);
    expect(JSON.parse(localStorage.getItem(`${key2}-sizes`) || '{}')).toEqual(
      sizes2,
    );
  });

  it('should debounce writes to localStorage', async () => {
    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    act(() => {
      result.current.onColumnSizingChange({ col1: 100 });
    });

    // Verify no immediate write
    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toBeNull();

    // Advance time a bit, still no write
    await act(async () => {
      await jest.advanceTimersByTimeAsync(300);
    });
    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toBeNull();

    // Another update resets/clears the previous timer
    act(() => {
      result.current.onColumnSizingChange({ col1: 150 });
    });

    // Advance another 300ms (total 600ms since first update, but only 300ms since second)
    await act(async () => {
      await jest.advanceTimersByTimeAsync(300);
    });
    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toBeNull();

    // Advance another 200ms to complete the 500ms debounce of the second update
    await act(async () => {
      await jest.advanceTimersByTimeAsync(200);
    });
    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toEqual(
      JSON.stringify({ col1: 150 }),
    );
  });

  it('should clear localStorage and reset columnSizing state on resetColumnWidths callback', () => {
    const storedSizes = { col1: 150, col2: 300 };
    localStorage.setItem(
      `${localStorageKey}-sizes`,
      JSON.stringify(storedSizes),
    );

    const { result } = renderHook(
      () =>
        useFleetMRTState({
          setSearchParams: mockSetSearchParams,
          pagerCtx: mockPagerCtx,
          filterValues: mockFilterValues,
          filterOptionsConfig: mockFilterOptionsConfig,
          visibleColumns,
          localStorageKey,
          defaultColumnIds,
          platform: Platform.CHROMEOS,
        }),
      { wrapper },
    );

    expect(result.current.columnSizing).toEqual(storedSizes);

    act(() => {
      result.current.resetColumnWidths();
    });

    expect(result.current.columnSizing).toEqual({});
    expect(localStorage.getItem(`${localStorageKey}-sizes`)).toBeNull();
  });
});
