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

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidDevicesPage } from './android_devices_page';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

// Stub background admin tasks alert polling to prevent unneeded network requests
jest.mock('../common/admin_tasks_alert', () => ({
  AdminTasksAlert: () => null,
}));

describe('<AndroidDevicesPage /> Integration', () => {
  const mockHealthResponse = {
    totalDevices: 100,
    totalHosts: 10,
    labRunningHosts: 8,
    labMissingHosts: 2,
    average7d: 0.842,
    average30d: 0.815,
    healthCategoryInService: {
      total: 70,
      statusCounts: { IDLE: 50, BUSY: 20 },
      average7d: 0.912,
      average30d: 0.895,
    },
    healthCategoryInTransition: {
      total: 10,
      statusCounts: { INIT: 5, PREPPING: 5 },
    },
    healthCategoryInAutoRecovery: {
      total: 10,
      statusCounts: { DIRTY: 10 },
    },
    healthCategoryNeedManualRepair: {
      total: 10,
      statusCounts: { MISSING: 5, FAILED: 5 },
    },
    healthCategoryUnspecified: {
      total: 0,
      statusCounts: {},
    },
  };

  const mockListResponse = {
    devices: [
      {
        id: 'dev-1',
        runTarget: 'pixel8',
        realm: 'test-realm',
        healthCategory: 'HEALTH_CATEGORY_IN_SERVICE',
      },
      {
        id: 'dev-2',
        runTarget: 'pixel9',
        realm: 'test-realm',
        healthCategory: 'HEALTH_CATEGORY_NEED_MANUAL_REPAIR',
      },
    ],
    totalSize: 2,
    nextPageToken: '',
  };

  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();

    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              totalHosts: 10,
              idleDevices: 50,
              busyDevices: 20,
              labRunningHosts: 8,
              labMissingHosts: 2,
              average7d: 0.842,
              average30d: 0.815,
            },
          }),
        }),
      },
      ListAndroidDevices: {
        query: () => ({
          queryKey: ['ListAndroidDevices'],
          queryFn: async () => mockListResponse,
        }),
      },
      GetDeviceDimensions: {
        query: () => ({
          queryKey: ['GetDeviceDimensions'],
          queryFn: async () => ({ baseDimensions: {}, labels: {} }),
        }),
      },
    });
  });

  it('renders page with AndroidHealthSummaryHeader when feature flag is enabled', async () => {
    localStorage.setItem(
      'featureFlag:fleet-console:android-health-metrics',
      'on',
    );

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/android/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AndroidDevicesPage workspace="Android" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(await screen.findByText('In Service')).toBeInTheDocument();
    expect(screen.getByText('Need Manual Repair')).toBeInTheDocument();
    expect(screen.getByText('In Automated Maintenance')).toBeInTheDocument();
  });

  it('renders utilization metrics when utilization flag is enabled', async () => {
    localStorage.setItem(
      'featureFlag:fleet-console:android-health-metrics',
      'on',
    );
    localStorage.setItem(
      'featureFlag:fleet-console:android-utilization-metrics',
      'on',
    );

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/android/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AndroidDevicesPage workspace="Android" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    // Total Devices utilization is shown
    expect(await screen.findByText('84.20%')).toBeInTheDocument();
    expect(screen.getByText('81.50%')).toBeInTheDocument();
  });

  it('renders legacy summary header when android-health-metrics flag is disabled', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/android/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AndroidDevicesPage workspace="Android" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(await screen.findByText('Online')).toBeInTheDocument();
    expect(screen.getByText('Offline')).toBeInTheDocument();
  });

  it('filters by category when clicking In Service metric on health header', async () => {
    localStorage.setItem(
      'featureFlag:fleet-console:android-health-metrics',
      'on',
    );

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/android/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AndroidDevicesPage workspace="Android" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const inServiceBtn = await screen.findByRole('button', {
      name: /In Service/i,
    });
    fireEvent.click(inServiceBtn);

    expect(screen.getByText('In Service')).toBeInTheDocument();
  });
});
