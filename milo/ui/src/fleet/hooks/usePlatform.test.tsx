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

import { renderHook } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import { usePlatform } from './usePlatform';

describe('usePlatform', () => {
  it('returns true when the platform is ANDROID', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs/android']}>
        <Routes>
          <Route path="/repairs/:platform" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => usePlatform(), { wrapper });
    expect(result.current.isValid).toBe(true);
  });

  it('returns false when the platform is not in the URL', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs']}>
        <Routes>
          <Route path="/repairs" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => usePlatform(), { wrapper });
    expect(result.current.isValid).toBe(false);
  });

  it('returns false when the platform is invalid', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs/invalid']}>
        <Routes>
          <Route path="/repairs/:platform" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => usePlatform(), { wrapper });
    expect(result.current.isValid).toBe(false);
  });

  it('currentPageSupportsPlatforms is false when the platform is not in the URL', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/repairs']}>
        <Routes>
          <Route path="/repairs" element={children} />
        </Routes>
      </MemoryRouter>
    );
    const { result } = renderHook(() => usePlatform(), { wrapper });
    expect(result.current.currentPageSupportsPlatforms).toBe(false);
  });
});
