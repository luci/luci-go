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
import { MemoryRouter, Route, Routes } from 'react-router';

import { useFleetAnalytics } from './use_fleet_analytics';

const mockTrackEvent = jest.fn();

jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({
    trackEvent: mockTrackEvent,
  }),
}));

describe('useFleetAnalytics', () => {
  beforeEach(() => {
    mockTrackEvent.mockClear();
  });

  it('attaches current platform parameter to tracked event payload', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/p/android/devices']}>
        <Routes>
          <Route path="/p/:platform/*" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFleetAnalytics(), { wrapper });
    result.current.trackEvent('export_csv', {
      componentName: 'export_csv_button',
    });

    expect(mockTrackEvent).toHaveBeenCalledWith('export_csv', {
      platform: 'android',
      componentName: 'export_csv_button',
    });
  });

  it('allows payload to explicitly override the inferred platform', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/p/android/devices']}>
        <Routes>
          <Route path="/p/:platform/*" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFleetAnalytics(), { wrapper });
    result.current.trackEvent('export_csv', {
      platform: 'custom_override',
      componentName: 'export_csv_button',
    });

    expect(mockTrackEvent).toHaveBeenCalledWith('export_csv', {
      platform: 'custom_override',
      componentName: 'export_csv_button',
    });
  });

  it('tracks event without payload when payload is omitted', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/p/chromeos/devices']}>
        <Routes>
          <Route path="/p/:platform/*" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFleetAnalytics(), { wrapper });
    result.current.trackEvent('page_view');

    expect(mockTrackEvent).toHaveBeenCalledWith('page_view', {
      platform: 'chromeos',
    });
  });

  it('handles pages outside of platform route cleanly', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/catalog']}>
        <Routes>
          <Route path="/catalog" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFleetAnalytics(), { wrapper });
    result.current.trackEvent('catalog_view', { componentName: 'table' });

    expect(mockTrackEvent).toHaveBeenCalledWith('catalog_view', {
      platform: undefined,
      componentName: 'table',
    });
  });

  it('maintains stable reference for returned hook object across renders', () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/p/android/devices']}>
        <Routes>
          <Route path="/p/:platform/*" element={<>{children}</>} />
        </Routes>
      </MemoryRouter>
    );

    const { result, rerender } = renderHook(() => useFleetAnalytics(), {
      wrapper,
    });
    const firstResult = result.current;

    rerender();

    expect(result.current).toBe(firstResult);
    expect(result.current.trackEvent).toBe(firstResult.trackEvent);
  });
});
