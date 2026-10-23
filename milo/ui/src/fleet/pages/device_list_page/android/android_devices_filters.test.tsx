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

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useAndroidFilters } from './use_android_filters';

jest.mock('@/fleet/pages/device_list_page/common/use_device_dimensions');

describe('normalizeFilterKey', () => {
  it('should strip labels. prefix and quotes', () => {
    expect(normalizeFilterKey('labels."build"')).toBe('build');
    expect(normalizeFilterKey('labels.build')).toBe('build');
    expect(normalizeFilterKey('build')).toBe('build');
  });

  it('should handle unquoted keys', () => {
    expect(normalizeFilterKey('host_group')).toBe('host_group');
  });
});

describe('Android Devices Table Filtering', () => {
  it('should verify that we can normalize keys consistently', () => {
    const keys = ['labels."build"', 'host_group', 'hostname'];
    const normalized = keys.map(normalizeFilterKey);
    expect(normalized).toEqual(['build', 'host_group', 'hostname']);
  });

  it('does not create a duplicate Offline Since filter category when fc_offline_since is returned in labels', () => {
    (useDeviceDimensions as jest.Mock).mockReturnValue({
      data: {
        baseDimensions: {
          hostname: { values: ['android-lab-host-01'] },
        },
        labels: {
          fc_offline_since: { values: ['2026-09-24T20:00:00Z'] },
          'ufs.last_sync': { values: ['2026-09-24T21:30:00Z'] },
          model: { values: ['husky'] },
        },
      },
      isLoading: false,
    });

    const { result } = renderHook(() => useAndroidFilters('Android'), {
      wrapper: ({ children }) => (
        <FakeContextProvider
          mountedPath="/ui/fleet/p/android/devices"
          routerOptions={{
            initialEntries: [
              '/ui/fleet/p/android/devices?filters=NOT+%22fc_offline_since%22%3A*',
            ],
          }}
        >
          {children}
        </FakeContextProvider>
      ),
    });

    const filterValues = result.current.filterValues ?? {};
    expect(filterValues['"fc_offline_since"']?.label).toBe('Offline Since');
    expect(filterValues['labels."fc_offline_since"']).toBeUndefined();
    const activeCategories = Object.values(filterValues).filter((c) =>
      c.isActive(),
    );
    expect(activeCategories).toHaveLength(1);
    expect(activeCategories[0].label).toBe('Offline Since');
  });
});
