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

import { act, renderHook } from '@testing-library/react';
import { useLocalStorage } from 'react-use';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  getInitialValue,
  getNewStates,
  synchSearchParamToLocalStorage,
  useParamsAndLocalStorage,
} from './use_params_and_local_storage';

// Mock dependencies
jest.mock('@/generic_libs/hooks/synced_search_params', () => ({
  useSyncedSearchParams: jest.fn(),
}));

jest.mock('react-use', () => ({
  useLocalStorage: jest.fn(),
}));

describe('useParamsAndLocalStorage', () => {
  describe('getInitialValue', () => {
    it('Should priorities search parameters', () => {
      const out = getInitialValue(
        new URLSearchParams([
          ['c', 'col1'],
          ['c', 'col2'],
          ['c', 'col3'],
        ]),
        'c',
        ['col8', 'col9'],
        ['default'],
      );

      expect(out).toEqual(['col1', 'col2', 'col3']);
    });
    it('Should priorities local storage after search parameters', () => {
      const out = getInitialValue(
        new URLSearchParams([]),
        'c',
        ['col8', 'col9'],
        ['default'],
      );

      expect(out).toEqual(['col8', 'col9']);
    });
    it('Should fall back to the default values', () => {
      const out1 = getInitialValue(new URLSearchParams([]), 'c', undefined, [
        'default',
      ]);

      expect(out1).toEqual(['default']);

      const out2 = getInitialValue(
        new URLSearchParams([]),
        'c',
        [],
        ['default'],
      );

      expect(out2).toEqual(['default']);
    });
  });

  describe('synchSearchParamToLocalStorage', () => {
    it('Should not do anything if query params are set', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['potato'],
        setSearchParams,
        ['default'],
      );

      expect(setSearchParams).not.toHaveBeenCalled();
    });
    it('Should not do anything if local storage is not set', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['c', 'col1']]),
        'c',
        undefined,
        setSearchParams,
        ['default'],
      );

      expect(setSearchParams).not.toHaveBeenCalled();
    });
    it('Should not do anything if local storage are set to the default', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['default'],
        setSearchParams,
        ['default'],
      );

      expect(setSearchParams).not.toHaveBeenCalled();
    });
    it('Should set query parameters if local storage is set and they are not', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([]),
        'c',
        ['potato'],
        setSearchParams,
        ['default'],
      );

      expect(setSearchParams).toHaveBeenCalledWith(
        new URLSearchParams([['c', 'potato']]),
      );
    });
    it('Should leave other query parameters unchanged', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['other', 'value']]),
        'c',
        ['potato'],
        setSearchParams,
        ['default'],
      );

      expect(setSearchParams).toHaveBeenCalledWith(
        new URLSearchParams([
          ['other', 'value'],
          ['c', 'potato'],
        ]),
      );
    });
  });

  describe('getNewStates', () => {
    it('Should set a new state', () => {
      const out = getNewStates(
        ['new', 'values'],
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: ['new', 'values'],
        searchParams: new URLSearchParams([
          ['c', 'new'],
          ['c', 'values'],
        ]),
      });
    });
    it('Should delete the state if setting to the default value', () => {
      const out = getNewStates(
        ['default'],
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: undefined,
        searchParams: new URLSearchParams([]),
      });
    });
    it('Should leave other query parameters unchanged', () => {
      const out = getNewStates(
        ['default'],
        new URLSearchParams([
          ['c', 'col1'],
          ['other', 'value'],
        ]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: undefined,
        searchParams: new URLSearchParams([['other', 'value']]),
      });
    });
  });
});

describe('useParamsAndLocalStorage Hook', () => {
  let mockSetSearchParams: jest.Mock;
  let mockSetLocalStorage: jest.Mock;
  let mockClearLocalStorage: jest.Mock;

  beforeEach(() => {
    mockSetSearchParams = jest.fn();
    mockSetLocalStorage = jest.fn();
    mockClearLocalStorage = jest.fn();

    (useSyncedSearchParams as jest.Mock).mockReturnValue([
      new URLSearchParams(),
      mockSetSearchParams,
    ]);

    (useLocalStorage as jest.Mock).mockReturnValue([
      undefined,
      mockSetLocalStorage,
      mockClearLocalStorage,
    ]);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('Example Scenario: Race Condition (Stale URL Update)', () => {
    // 1. Initial State
    // URL: []
    // LocalStorage: undefined
    // Default: ['default']
    const initParams = new URLSearchParams();

    // Setup a mechanism to allow tests to update the mocked hook return values
    // and trigger re-renders.
    let currentParams = initParams;
    const currentLocalStorage = undefined;

    (useSyncedSearchParams as jest.Mock).mockImplementation(() => [
      currentParams,
      mockSetSearchParams,
    ]);
    (useLocalStorage as jest.Mock).mockImplementation(() => [
      currentLocalStorage,
      mockSetLocalStorage,
      mockClearLocalStorage,
    ]);

    const { result, rerender } = renderHook(() =>
      useParamsAndLocalStorage('c', 'ls-key', ['default']),
    );

    // Initial check
    const [cols] = result.current;
    expect(cols).toEqual(['default']);

    // 2. User updates columns to ['A']
    const [, setCols] = result.current;
    act(() => {
      setCols(['A']);
    });

    // Verify optimistic update (CHECK 1: Optimistic update works)
    expect(result.current[0]).toEqual(['A']);
    expect(mockSetSearchParams).toHaveBeenCalled();

    // 3. RACE CONDITION SIMULATION:
    // Force a re-render while the external hook (useSyncedSearchParams) still returns the OLD value.
    // This simulates the gap between calling setSearchParams and the URL actually updating.
    //
    // Notes for maintainers:
    // This tests that the hook ignores stale updates from the URL.
    // If the hook implemented a naive synchronization (e.g. syncing from URL on every render),
    // this re-render would revert the state back to ['default'] because the URL hasn't been updated in the mock yet.
    //
    // The previous implementation had a bug where this re-render would revert 'syncedState' to ['default'].
    rerender();

    // Verify state is STILL ['A'] (robustness check)
    expect(result.current[0]).toEqual(['A']);

    // 4. Finally, URL updates to ['A']
    currentParams = new URLSearchParams([['c', 'A']]);
    rerender();
    expect(result.current[0]).toEqual(['A']);
  });

  it('Example Scenario: External URL Update', () => {
    // 1. Initial State
    let currentParams = new URLSearchParams([['c', 'A']]);
    (useSyncedSearchParams as jest.Mock).mockImplementation(() => [
      currentParams,
      mockSetSearchParams,
    ]);

    const { result, rerender } = renderHook(() =>
      useParamsAndLocalStorage('c', 'ls-key', ['default']),
    );

    expect(result.current[0]).toEqual(['A']);

    // 2. External Navigation (e.g. user clicks back button) -> URL changes to ['B']
    currentParams = new URLSearchParams([['c', 'B']]);
    rerender();

    // Verify state updates to match URL
    expect(result.current[0]).toEqual(['B']);
  });
});
