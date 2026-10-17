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

import { render, screen, fireEvent } from '@testing-library/react';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BrowserSummaryHeader } from './browser_summary_header';

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

describe('BrowserSummaryHeader', () => {
  beforeEach(() => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();
    jest.clearAllMocks();
  });

  const setupMockClient = (dataMap: Record<string, number>) => {
    const total = dataMap.total || 1000;
    const healthy = dataMap.healthy || 800;
    const dead = dataMap.dead || 50;
    const quarantined = dataMap.quarantined || 30;
    const maintenance = dataMap.maintenance || 20;

    FleetConsoleMockAPI.setFixture('CountBrowserDevices', {
      total,
      swarmingState: {
        total: healthy + dead + quarantined + maintenance,
        alive: healthy,
        dead,
        quarantined,
        maintenance,
      },
    });

    FleetConsoleMockAPI.setFixture('GetBrowserDeviceDimensions', {
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

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <BrowserSummaryHeader />
      </FakeContextProvider>,
    );

    // Verify title and main scorecards
    expect(
      await screen.findByText('Device Health Summary'),
    ).toBeInTheDocument();
    expect((await screen.findAllByText('1,000')).length).toBeGreaterThan(0); // total Devices
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

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <BrowserSummaryHeader />
      </FakeContextProvider>,
    );

    // Wait for stats to load first
    expect((await screen.findAllByText('1,000')).length).toBeGreaterThan(0);

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

  it('renders exact non-overlapping counts with multi-dimensional group_by records', async () => {
    FleetConsoleMockAPI.setFixture('CountBrowserDevices', {
      total: 1000,
      records: [
        {
          dimensions: {
            'ufs.resource_state': 'SERVING',
            bot_health_status: 'HEALTHY',
          },
          count: '780',
        },
        {
          dimensions: {
            'ufs.resource_state': 'SERVING',
            bot_health_status: 'DEAD',
          },
          count: '40',
        },
        {
          dimensions: {
            'ufs.resource_state': 'SERVING',
            bot_health_status: 'QUARANTINED',
          },
          count: '25',
        },
        {
          dimensions: {
            'ufs.resource_state': 'SERVING',
            bot_health_status: 'MAINTENANCE',
          },
          count: '15',
        },
        {
          dimensions: {
            'ufs.resource_state': 'SERVING',
            bot_health_status: 'MISSING',
          },
          count: '20',
        },
        {
          dimensions: {
            'ufs.resource_state': 'NEEDS_REPAIR',
            bot_health_status: 'MISSING',
          },
          count: '10',
        },
        {
          dimensions: {
            'ufs.resource_state': 'MISSING',
            bot_health_status: 'MISSING',
          },
          count: '10',
        },
        {
          dimensions: {
            'ufs.resource_state': 'RESERVED',
            bot_health_status: 'MISSING',
          },
          count: '100',
        },
      ],
    });

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <BrowserSummaryHeader />
      </FakeContextProvider>,
    );

    expect(
      await screen.findByText('Device Health Summary'),
    ).toBeInTheDocument();
    expect((await screen.findAllByText('1,000')).length).toBeGreaterThan(0);
    expect(await screen.findAllByText('780')).toHaveLength(2); // healthy top-level + submetric
    expect(await screen.findAllByText('100')).toHaveLength(2); // unhealthy (80 swarming + 20 ufs)
    expect(await screen.findAllByText('120')).toHaveLength(1); // other (100 reserved + 20 missing bot)
  });
});
