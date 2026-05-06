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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { ReactNode } from 'react';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { fakeUseSyncedSearchParams } from '@/fleet/testing_tools/mocks/fake_search_params';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { GetResourceRequestsMultiselectFilterValuesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useRriFilters } from './use_rri_filters';

const MOCK_FILTER_VALUES =
  GetResourceRequestsMultiselectFilterValuesResponse.fromPartial({
    rrIds: ['rr-id-1', 'rr-id-2', 'abc-123', 'xyz-123'],
  });

// TODO: b/435182355 - Look into patterns for improving network request mocking.
jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(() => ({
    GetResourceRequestsMultiselectFilterValues: {
      query: jest.fn(),
    },
  })),
}));

describe('useRriFilters', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    fakeUseSyncedSearchParams();
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      GetResourceRequestsMultiselectFilterValues: {
        query: () => ({
          queryKey: ['test'],
          queryFn: () => MOCK_FILTER_VALUES,
        }),
      },
    });
  });

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  it('should set and parse filters from search params', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'rr_id="rr-id-1" OR rr_id="rr-id-2"');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });

    await waitFor(() => {
      const rrIdFilter = result.current.filterValues
        ?.rr_id as StringListFilterCategory;
      expect(rrIdFilter?.getOptions()['"rr-id-1"']?.isSelected).toBe(true);
    });

    const rrIdFilter = result.current.filterValues
      ?.rr_id as StringListFilterCategory;
    expect(rrIdFilter.getOptions()['"rr-id-2"'].isSelected).toBe(true);
  });

  it('should generate an AIP string from filters', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'rr_id="rr-id-1" OR rr_id="rr-id-2"');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.aipString).toEqual(
        '(rr_id = "rr-id-1" OR rr_id = "rr-id-2")',
      ),
    );
  });

  it('should return the correct label for a filter', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'rr_id="rr-id-1" OR rr_id="rr-id-2"');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.filterValues?.rr_id.getChipLabel()).toEqual(
        '2 | RR ID IN rr-id-1, rr-id-2',
      ),
    );
  });

  it('should generate an AIP string for range filters like slippage', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'slippage >= 5 AND slippage <= 10');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.aipString).toEqual(
        'slippage >= 5 AND slippage <= 10',
      ),
    );
  });

  it('should return the correct label for a range filter like slippage', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'slippage >= 5 AND slippage <= 10');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.filterValues?.slippage.getChipLabel()).toEqual(
        '[ Slippage ]: from 5 to 10',
      ),
    );
  });

  it('should handle undefined boundaries in RangeFilters without generating NaN', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set('filters', 'slippage <= 245');
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.aipString).toEqual('slippage <= 245'),
    );
  });

  it('should generate an AIP string for date filters with date-only format', async () => {
    const { result: searchParamsResult } = renderHook(
      () => useSyncedSearchParams(),
      { wrapper },
    );
    act(() => {
      const sp = new URLSearchParams();
      sp.set(
        'filters',
        'material_sourcing_actual_delivery_date >= "2022-01-01" AND material_sourcing_actual_delivery_date <= "2022-01-02"',
      );
      searchParamsResult.current[1](sp);
    });

    const { result } = renderHook(() => useRriFilters(), { wrapper });
    await waitFor(() =>
      expect(result.current.aipString).toEqual(
        'material_sourcing_actual_delivery_date >= "2022-01-01" AND material_sourcing_actual_delivery_date <= "2022-01-02"',
      ),
    );
  });
});
