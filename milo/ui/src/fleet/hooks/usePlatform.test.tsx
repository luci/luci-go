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
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';

import * as FeatureFlagsModule from '@/common/feature_flags';
import {
  enableAndroidRepairQueue,
  enableAndroidWorkforceActivity,
} from '@/fleet/features';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useIsInPlatformScope, usePlatform } from './usePlatform';

describe('usePlatform', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('returns the correct platform when the platform is valid', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs/android']}>
        <Routes>
          <Route path="/repairs/:platform" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => usePlatform(), { wrapper });
    expect(result.current.platform).toBe(Platform.ANDROID);
  });

  it('returns undefined when the platform is not in the URL', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs']}>
        <Routes>
          <Route path="/repairs" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => useIsInPlatformScope(), { wrapper });
    expect(result.current).toBe(false);
  });

  it('returns undefined when the platform is invalid', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs/invalid']}>
        <Routes>
          <Route path="/repairs/:platform" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => useIsInPlatformScope(), { wrapper });
    expect(result.current).toBe(false);
  });

  it('inPlatformScope is false when the platform is not in the URL', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs']}>
        <Routes>
          <Route path="/repairs" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => useIsInPlatformScope(), { wrapper });
    expect(result.current).toBe(false);
  });

  it('switches ChromeOS workforce to Android workforce only when both repair queue and workforce flags are enabled', () => {
    window.history.replaceState(
      null,
      '',
      '/ui/fleet/p/chromeos/repairs/workforce',
    );
    const flagSpy = jest
      .spyOn(FeatureFlagsModule, 'getFeatureFlagValue')
      .mockImplementation(
        (flag) =>
          flag === enableAndroidRepairQueue ||
          flag === enableAndroidWorkforceActivity,
      );

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/ui/fleet/p/chromeos/repairs/workforce']}>
        <Routes>
          <Route path="/ui/fleet/p/:platform/*" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(
      () => ({ platform: usePlatform(), location: useLocation() }),
      { wrapper },
    );

    act(() => {
      result.current.platform.setPlatform(Platform.ANDROID);
    });
    expect(result.current.location.pathname).toBe(
      '/ui/fleet/p/android/repairs/workforce',
    );

    flagSpy.mockImplementation((flag) => flag === enableAndroidRepairQueue);
    act(() => {
      result.current.platform.setPlatform(Platform.PIXEL);
    });
    expect(result.current.location.pathname).toBe('/ui/fleet/p/pixel/repairs');
  });
});
