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
import { useLocation } from 'react-router';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  DEFAULT_REPAIR_LAB,
  REPAIR_LAB_LOCAL_STORAGE_KEY,
  useRepairLab,
} from './use_repair_lab';

describe('useRepairLab', () => {
  beforeEach(() => {
    localStorage.clear();
    jest.clearAllMocks();
  });

  it('defaults to MTV when search param is unset or invalid', () => {
    const { result: defaultResult } = renderHook(() => useRepairLab(), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/chromeos/repairs"
          routerOptions={{
            initialEntries: ['/ui/fleet/p/chromeos/repairs'],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    expect(defaultResult.current.lab).toBe(DEFAULT_REPAIR_LAB);
    expect(localStorage.getItem(REPAIR_LAB_LOCAL_STORAGE_KEY)).toBe('MTV');

    const { result: invalidResult } = renderHook(() => useRepairLab(), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/chromeos/repairs"
          routerOptions={{
            initialEntries: ['/ui/fleet/p/chromeos/repairs?lab=INVALID_LAB'],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    expect(invalidResult.current.lab).toBe(DEFAULT_REPAIR_LAB);
  });

  it('reads lab from URL search param', () => {
    const { result } = renderHook(() => useRepairLab(), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/chromeos/repairs"
          routerOptions={{
            initialEntries: ['/ui/fleet/p/chromeos/repairs?lab=HTL'],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    expect(result.current.lab).toBe('HTL');
    expect(localStorage.getItem(REPAIR_LAB_LOCAL_STORAGE_KEY)).toBe('HTL');
  });

  it('reads lab from localStorage when search param is not provided', () => {
    localStorage.setItem(REPAIR_LAB_LOCAL_STORAGE_KEY, 'HTL');

    const { result } = renderHook(() => useRepairLab(), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/chromeos/repairs"
          routerOptions={{
            initialEntries: ['/ui/fleet/p/chromeos/repairs'],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    expect(result.current.lab).toBe('HTL');
  });

  it('updates both localStorage and URL search params when setLab is called', () => {
    const { result } = renderHook(() => useRepairLab(), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/chromeos/repairs"
          routerOptions={{
            initialEntries: ['/ui/fleet/p/chromeos/repairs?lab=MTV'],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    expect(result.current.lab).toBe('MTV');

    act(() => {
      result.current.setLab('HTL');
    });

    expect(result.current.lab).toBe('HTL');
    expect(localStorage.getItem(REPAIR_LAB_LOCAL_STORAGE_KEY)).toBe('HTL');
  });

  it('clears cursor pagination param when switching labs', () => {
    const { result } = renderHook(
      () => ({
        repairLab: useRepairLab(),
        location: useLocation(),
      }),
      {
        wrapper: ({ children }) => (
          <FakeContextProvider
            mountedPath="/ui/fleet/p/chromeos/repairs"
            routerOptions={{
              initialEntries: [
                '/ui/fleet/p/chromeos/repairs?lab=MTV&cursor=page2token',
              ],
            }}
          >
            {children}
          </FakeContextProvider>
        ),
      },
    );

    expect(result.current.location.search).toContain('cursor=page2token');

    act(() => {
      result.current.repairLab.setLab('HTL');
    });

    expect(result.current.repairLab.lab).toBe('HTL');
    expect(result.current.location.search).toContain('lab=HTL');
    expect(result.current.location.search).not.toContain('cursor=');
  });
});
