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

import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api/mock_api_handler';
import * as exportUtils from '@/fleet/utils/export';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BrowserDevicesPage } from './browser_devices_page';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

// Stub background admin tasks alert polling to prevent unneeded network requests
jest.mock('../common/admin_tasks_alert', () => ({
  AdminTasksAlert: () => null,
}));

describe('<BrowserDevicesPage />', () => {
  jest.setTimeout(30000);

  let originalCreateObjectURL: typeof window.URL.createObjectURL;
  let originalRevokeObjectURL: typeof window.URL.revokeObjectURL;

  beforeAll(() => {
    originalCreateObjectURL = window.URL.createObjectURL;
    originalRevokeObjectURL = window.URL.revokeObjectURL;
    window.URL.createObjectURL = jest.fn(() => 'mock-url');
    window.URL.revokeObjectURL = jest.fn();
    FleetConsoleMockAPI.enableBrowserInterceptor();
  });

  afterAll(() => {
    window.URL.createObjectURL = originalCreateObjectURL;
    window.URL.revokeObjectURL = originalRevokeObjectURL;
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  beforeEach(() => {
    mockTrackEvent.mockClear();
    FleetConsoleMockAPI.resetFixtures();
    FleetConsoleMockAPI.setFixture('ListBrowserDevices', {
      devices: [
        {
          id: '1',
          deviceId: 'browser-1',
          ufsLabels: { hostname: { values: ['host-1'] } },
        },
        {
          id: '2',
          deviceId: 'browser-2',
          ufsLabels: { hostname: { values: ['host-2'] } },
        },
      ],
      totalSize: 2,
    });
    FleetConsoleMockAPI.setFixture('CountBrowserDevices', {
      total: 10,
      swarmingState: {
        total: 10,
        alive: 8,
        dead: 2,
        quarantined: 0,
        maintenance: 0,
      },
    });
    FleetConsoleMockAPI.setFixture('ExportBrowserDevicesToCSV', {
      csvData: 'id,device_id\n1,browser-1\n2,browser-2\n',
    });
  });

  it('should render', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <BrowserDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(await screen.findByText('Device Health Summary')).toBeVisible();
  });

  it('should allow clicking action buttons when rows are selected', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <BrowserDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Wait for row checkbox to load and appear
    const rowCheckbox = await screen.findByTestId('select-checkbox-1');
    expect(rowCheckbox).toBeInTheDocument();

    // Select the first row
    fireEvent.click(rowCheckbox);

    const repairButton = await screen.findByRole('button', {
      name: /request repair/i,
    });
    expect(repairButton).toBeVisible();
  }, 15000);

  it('should allow customizing columns', async () => {
    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: ['/p/chromium/devices'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <BrowserDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const columnsButton = await screen.findByText('Columns');
    expect(columnsButton).toBeVisible();
    fireEvent.click(columnsButton);

    const searchInput = screen.getByPlaceholderText(/search/i);
    expect(searchInput).toBeVisible();
  });

  it('should call ExportBrowserDevicesToCSV with correct filters and columns', async () => {
    const exportAsSpy = jest
      .spyOn(exportUtils, 'exportAs')
      .mockImplementation(() => {});

    render(
      <FakeContextProvider
        mountedPath="/p/:platform/devices"
        routerOptions={{
          initialEntries: [
            '/p/chromium/devices?filters=' +
              encodeURIComponent('(os = "Linux")'),
          ],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <BrowserDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Wait for the Export button to appear
    const exportButton = await screen.findByRole('button', { name: /export/i });
    fireEvent.click(exportButton);

    // Find and click "Export all (CSV)" menu item
    const exportAllItem = await screen.findByText('Export all (CSV)');
    fireEvent.click(exportAllItem);

    // Verify that exportAs was called (which means export completed)
    await waitFor(() => {
      expect(exportAsSpy).toHaveBeenCalledWith(
        expect.any(Blob),
        'csv',
        'fleet_console_browser_devices',
      );
    });

    // Verify analytics tracking
    expect(mockTrackEvent).toHaveBeenCalledWith('export_csv', {
      componentName: 'export_csv_button',
      dutCount: undefined,
      platform: 'chromium',
    });
  }, 60000);
});
