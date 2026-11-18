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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';

import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import {
  CountAndroidDevicesRequest,
  CountDevicesRequest,
  PhysicalSite,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  usePhysicalSites,
  usePlatformSiteBrokenCounts,
} from './use_physical_sites';

describe('use_physical_sites hooks', () => {
  let queryClient: QueryClient;
  let mockListPhysicalSites: jest.Mock;
  let mockCountDevices: jest.Mock;
  let mockCountAndroidDevices: jest.Mock;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    mockListPhysicalSites = jest.fn();
    mockCountDevices = jest.fn();
    mockCountAndroidDevices = jest.fn();

    const mockClient = {
      ListPhysicalSites: {
        query: (req: Record<string, unknown>) => ({
          queryKey: ['ListPhysicalSites', req],
          queryFn: () => mockListPhysicalSites(req),
        }),
      },
      CountDevices: {
        query: (req: Partial<CountDevicesRequest>) => ({
          queryKey: ['CountDevices', req.platform, req.filter ?? ''],
          queryFn: () => mockCountDevices(req),
        }),
      },
      CountAndroidDevices: {
        query: (req: Partial<CountAndroidDevicesRequest>) => ({
          queryKey: ['CountAndroidDevices', req.filter ?? ''],
          queryFn: () => mockCountAndroidDevices(req),
        }),
      },
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>;

    jest
      .spyOn(PrpcClientsModule, 'useFleetConsoleClient')
      .mockReturnValue(mockClient);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  it('does not fetch ListPhysicalSites when usePhysicalSites({ enabled: false })', async () => {
    mockListPhysicalSites.mockResolvedValue({
      physicalSites: [
        PhysicalSite.fromPartial({ id: 'SITE-ALPHA', name: 'Alpha Lab' }),
      ],
    });

    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => usePhysicalSites({ enabled }),
      { wrapper, initialProps: { enabled: false } },
    );

    expect(mockListPhysicalSites).not.toHaveBeenCalled();
    expect(result.current.sites).toEqual([]);
    expect(result.current.isLoading).toBe(false);

    rerender({ enabled: true });

    await waitFor(() => expect(result.current.sites).toHaveLength(1));
    expect(mockListPhysicalSites).toHaveBeenCalledTimes(1);
    expect(result.current.sites[0].id).toBe('SITE-ALPHA');
  });

  it('composes baseFilter, deduplicates trimmed filters, and distinguishes null vs 0 for ChromeOS', async () => {
    // Seed warm unscoped cache entry to verify empty/whitespace filters never read it.
    queryClient.setQueryData(['CountDevices', Platform.CHROMEOS, ''], {
      deviceState: { needManualRepair: 999 },
    });
    queryClient.setQueryData(
      ['CountDevices', Platform.CHROMEOS, 'pool = "mock-pool"'],
      { deviceState: { needManualRepair: 888 } },
    );

    mockCountDevices.mockImplementation(
      async (req: Partial<CountDevicesRequest>) => {
        if (
          req.filter === '(pool = "mock-pool") AND (labels.zone = "zone-a")'
        ) {
          return { deviceState: { needManualRepair: 7 } };
        }
        if (
          req.filter === '(pool = "mock-pool") AND (labels.zone = "zone-zero")'
        ) {
          return { deviceState: { needManualRepair: 0 } };
        }
        throw new Error(`unexpected filter: ${req.filter}`);
      },
    );

    const sites = [
      { id: 'SITE-A1', filter: 'labels.zone = "zone-a"' },
      { id: 'SITE-A2', filter: '  labels.zone = "zone-a"  ' },
      { id: 'SITE-ZERO', filter: 'labels.zone = "zone-zero"' },
      { id: 'SITE-EMPTY', filter: '' },
      { id: 'SITE-WHITESPACE', filter: '   ' },
    ];

    const { result } = renderHook(
      () =>
        usePlatformSiteBrokenCounts(
          sites,
          Platform.CHROMEOS,
          'pool = "mock-pool"',
          true,
        ),
      { wrapper },
    );

    // Before resolution, all counts are null (distinct from 0).
    expect(result.current).toEqual({
      'SITE-A1': null,
      'SITE-A2': null,
      'SITE-ZERO': null,
      'SITE-EMPTY': null,
      'SITE-WHITESPACE': null,
    });

    await waitFor(() => expect(result.current['SITE-A1']).toBe(7));

    // Only 2 distinct non-empty filters should be queried despite 5 sites.
    expect(mockCountDevices).toHaveBeenCalledTimes(2);
    expect(mockCountDevices).toHaveBeenCalledWith({
      filter: '(pool = "mock-pool") AND (labels.zone = "zone-a")',
      platform: Platform.CHROMEOS,
    });
    expect(mockCountDevices).toHaveBeenCalledWith({
      filter: '(pool = "mock-pool") AND (labels.zone = "zone-zero")',
      platform: Platform.CHROMEOS,
    });
    expect(result.current).toEqual({
      'SITE-A1': 7,
      'SITE-A2': 7,
      'SITE-ZERO': 0,
      'SITE-EMPTY': null,
      'SITE-WHITESPACE': null,
    });
  });

  it('composes Android baseFilter and returns null without reading warm cache when enabled=false', async () => {
    const composedFilter =
      '(has(host_group, "mock-group")) AND (lab_name = "mock-lab")';
    queryClient.setQueryData(['CountAndroidDevices', composedFilter], {
      healthCategoryNeedManualRepair: { total: 42 },
    });
    mockCountAndroidDevices.mockResolvedValue({
      healthCategoryNeedManualRepair: { total: 12 },
    });

    const sites = [
      { id: 'SITE-ALPHA', filter: 'lab_name = "mock-lab"' },
      { id: 'SITE-BLANK', filter: '   ' },
    ];

    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        usePlatformSiteBrokenCounts(
          sites,
          Platform.ANDROID,
          'has(host_group, "mock-group")',
          enabled,
        ),
      { wrapper, initialProps: { enabled: false } },
    );

    // Warm QueryClient cache must not leak when enabled=false.
    expect(mockCountAndroidDevices).not.toHaveBeenCalled();
    expect(result.current).toEqual({
      'SITE-ALPHA': null,
      'SITE-BLANK': null,
    });

    rerender({ enabled: true });

    await waitFor(() => expect(result.current['SITE-ALPHA']).toBe(12));
    expect(mockCountAndroidDevices).toHaveBeenCalledTimes(1);
    expect(mockCountAndroidDevices).toHaveBeenCalledWith({
      filter: composedFilter,
    });
    expect(result.current).toEqual({
      'SITE-ALPHA': 12,
      'SITE-BLANK': null,
    });
  });
});
