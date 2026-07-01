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
import * as PrpcClients from '@/fleet/hooks/prpc_clients';
import * as exportUtils from '@/fleet/utils/export';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BrowserDevicesPage } from './browser_devices_page';

jest.mock('./use_browser_device_dimensions', () => {
  const mockData = {
    baseDimensions: {
      os: { values: ['Linux', 'Windows'] },
    },
    swarmingLabels: {},
    ufsLabels: {},
  };
  return {
    useBrowserDeviceDimensions: () => ({
      data: mockData,
      isPending: false,
    }),
  };
});

describe('<BrowserDevicesPage />', () => {
  afterEach(() => {
    jest.restoreAllMocks();
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

    expect(screen.getByText('Main metrics')).toBeVisible();
  });

  it('should call ExportBrowserDevicesToCSV with correct filters', async () => {
    const mockExport = jest
      .fn()
      .mockResolvedValue({ csvData: 'id,device_id\n1,browser-1\n' });

    const mockCountBrowserDevices = jest.fn().mockReturnValue({
      queryKey: ['CountBrowserDevices'],
      queryFn: jest.fn().mockResolvedValue({
        total: 10,
        swarmingState: {
          total: 10,
          alive: 8,
          dead: 2,
          quarantined: 0,
          maintenance: 0,
        },
      }),
    });

    const mockListBrowserDevices = jest.fn().mockReturnValue({
      queryKey: ['ListBrowserDevices'],
      queryFn: jest.fn().mockResolvedValue({
        devices: [
          { id: '1', deviceId: 'browser-1' },
          { id: '2', deviceId: 'browser-2' },
        ],
      }),
    });

    jest.spyOn(PrpcClients, 'useFleetConsoleClient').mockReturnValue({
      ExportBrowserDevicesToCSV: mockExport,
      CountBrowserDevices: {
        query: mockCountBrowserDevices,
      },
      ListBrowserDevices: {
        query: mockListBrowserDevices,
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);

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

    const exportAsSpy = jest
      .spyOn(exportUtils, 'exportAs')
      .mockImplementation(() => {});

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

    // Verify that the query was triggered with parsed and formatted filter keys
    expect(mockExport).toHaveBeenCalled();
    const callArgs = mockExport.mock.lastCall![0];
    expect(callArgs.filter).toBe('(os = "Linux")');
  });
});
