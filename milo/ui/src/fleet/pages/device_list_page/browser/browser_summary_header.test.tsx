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
import { render, screen, fireEvent } from '@testing-library/react';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BrowserSummaryHeader } from './browser_summary_header';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

describe('BrowserSummaryHeader', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const setupMockClient = (dataMap: Record<string, number>) => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
    mockUseFleetConsoleClient.mockReturnValue({
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(({ filter }) => {
          let total = 0;
          let swarmingState = undefined;

          if (!filter) {
            total = dataMap.total || 0;
            swarmingState = {
              total:
                (dataMap.healthy || 0) +
                (dataMap.dead || 0) +
                (dataMap.quarantined || 0) +
                (dataMap.maintenance || 0),
              alive: dataMap.healthy || 0,
              dead: dataMap.dead || 0,
              quarantined: dataMap.quarantined || 0,
              maintenance: dataMap.maintenance || 0,
            };
          } else if (
            filter.includes('ufs.resource_state = "SERVING"') ||
            filter.includes("ufs.resource_state = 'SERVING'")
          ) {
            total =
              (dataMap.healthy || 0) +
              (dataMap.dead || 0) +
              (dataMap.quarantined || 0) +
              (dataMap.maintenance || 0);
            swarmingState = {
              total: total,
              alive: dataMap.healthy || 0,
              dead: dataMap.dead || 0,
              quarantined: dataMap.quarantined || 0,
              maintenance: dataMap.maintenance || 0,
            };
          } else if (filter.includes('NEEDS_REPAIR')) {
            total = dataMap.needsRepair || 0;
          } else if (filter.includes('MISSING')) {
            total = dataMap.missing || 0;
          } else if (filter.includes('RESERVED')) {
            total = dataMap.excluded || 0;
          }

          return {
            queryKey: ['CountBrowserDevices', filter],
            queryFn: async () => ({
              total,
              swarmingState,
            }),
          };
        }),
      },
      GetBrowserDeviceDimensions: {
        query: () => ({
          queryKey: ['GetBrowserDeviceDimensions'],
          queryFn: async () => ({
            baseDimensions: {
              os: { values: ['Linux', 'Windows'] },
            },
            swarmingLabels: {
              state: {
                values: ['alive', 'dead', 'quarantined', 'maintenance'],
              },
            },
            ufsLabels: {
              resource_state: {
                values: ['SERVING', 'NEEDS_REPAIR', 'MISSING', 'RESERVED'],
              },
            },
          }),
        }),
      },
    });
  };

  it('renders stats correctly', async () => {
    setupMockClient({
      total: 1000,
      healthy: 800,
      dead: 50,
      quarantined: 30,
      maintenance: 20,
      needsRepair: 10,
      missing: 5,
      excluded: 85,
    });

    const queryClient = new QueryClient();
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <QueryClientProvider client={queryClient}>
          <BrowserSummaryHeader />
        </QueryClientProvider>
      </FakeContextProvider>,
    );

    // Verify title and main scorecards
    expect(
      await screen.findByText('Device Health Summary'),
    ).toBeInTheDocument();
    expect(await screen.findByText('1,000')).toBeInTheDocument(); // total Devices
    expect(await screen.findAllByText('750')).toHaveLength(2); // healthy
  });

  it('triggers filters on card and metric clicks', async () => {
    setupMockClient({
      total: 1000,
      healthy: 800,
      dead: 50,
      quarantined: 30,
      maintenance: 20,
      needsRepair: 10,
      missing: 5,
      excluded: 85,
    });

    const setSelectedOptionsSpy = jest
      .spyOn(StringListFilterCategory.prototype, 'setSelectedOptions')
      .mockImplementation(() => undefined);

    const queryClient = new QueryClient();
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <QueryClientProvider client={queryClient}>
          <BrowserSummaryHeader />
        </QueryClientProvider>
      </FakeContextProvider>,
    );

    // Wait for stats to load first
    expect(await screen.findByText('1,000')).toBeInTheDocument();

    // Click Unhealthy Header
    const unhealthyHeader = screen.getByText('Unhealthy');
    fireEvent.click(unhealthyHeader);
    expect(setSelectedOptionsSpy).toHaveBeenCalledWith([
      'dead',
      'quarantined',
      'maintenance',
      BLANK_VALUE,
    ]);
    expect(setSelectedOptionsSpy).toHaveBeenCalledWith([
      'SERVING',
      'NEEDS_REPAIR',
      'MISSING',
    ]);

    // Click In Maintenance item (under Swarming Bot Issues)
    const maintenanceItem = screen.getByText('In Maintenance:');
    fireEvent.click(maintenanceItem);
    expect(setSelectedOptionsSpy).toHaveBeenCalledWith(['maintenance']);
    expect(setSelectedOptionsSpy).toHaveBeenCalledWith(['SERVING']);
  });
});
