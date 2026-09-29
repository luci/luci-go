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

import { UseQueryResult } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';

import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HEALTH_FILTER_CONFIGS } from './filter_constants';
import { useHealthFilters } from './use_health_filters';

describe('useHealthFilters', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('provides model filter options from dimensionSource ("label-model")', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {},
          labels: {
            'label-model': { values: ['volteer', 'brya'] },
          },
        } as unknown as GetDeviceDimensionsResponse,
        isPending: false,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    const { result } = renderHook(() => useHealthFilters(), {
      wrapper: FakeContextProvider,
    });

    expect(result.current.filterValues).toBeDefined();
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.MODEL.key],
    ).toBeDefined();
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.MODEL.key].label,
    ).toBe(HEALTH_FILTER_CONFIGS.MODEL.label);
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.MODEL.key].key,
    ).toBe(HEALTH_FILTER_CONFIGS.MODEL.key);
    expect(result.current.isLoading).toBe(false);
  });

  it('provides pool filter options from dimensionSource ("label-pool")', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {},
          labels: {
            'label-pool': { values: ['DUT_POOL_QUOTA', 'DUT_POOL_CQ'] },
          },
        } as unknown as GetDeviceDimensionsResponse,
        isPending: false,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    const { result } = renderHook(() => useHealthFilters(), {
      wrapper: FakeContextProvider,
    });

    expect(result.current.filterValues).toBeDefined();
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.POOL.key],
    ).toBeDefined();
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.POOL.key].label,
    ).toBe(HEALTH_FILTER_CONFIGS.POOL.label);
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.POOL.key].key,
    ).toBe(HEALTH_FILTER_CONFIGS.POOL.key);
  });

  it('provides dynamic label filter options and supersedes core dimension sources', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {},
          labels: {
            'label-model': { values: ['volteer'] },
            'label-pool': { values: ['DUT_POOL_QUOTA'] },
            'label-board': { values: ['brya', 'volteer'] },
            dut_state: { values: ['ready', 'needs_repair'] },
          },
        } as unknown as GetDeviceDimensionsResponse,
        isPending: false,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    const { result } = renderHook(() => useHealthFilters(), {
      wrapper: FakeContextProvider,
    });

    expect(result.current.filterValues).toBeDefined();
    // Core filters present
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.MODEL.key],
    ).toBeDefined();
    expect(
      result.current.filterValues?.[HEALTH_FILTER_CONFIGS.POOL.key],
    ).toBeDefined();

    // Dynamic labels present with label key format
    expect(result.current.filterValues?.['labels."label-board"']).toBeDefined();
    expect(result.current.filterValues?.['labels."label-board"'].label).toBe(
      'Board',
    );
    expect(result.current.filterValues?.['labels."dut_state"']).toBeDefined();

    // Superseded labels should not appear as duplicated dynamic label filters
    expect(
      result.current.filterValues?.['labels."label-model"'],
    ).toBeUndefined();
    expect(
      result.current.filterValues?.['labels."label-pool"'],
    ).toBeUndefined();
  });

  it('indicates loading when dimensions are pending', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: undefined,
        isPending: true,
        isLoading: true,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    const { result } = renderHook(() => useHealthFilters(), {
      wrapper: FakeContextProvider,
    });

    expect(result.current.isLoading).toBe(true);
  });
});
