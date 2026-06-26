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

import { render, screen, fireEvent, act } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as PrpcClients from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDevicesPage } from './chromeos_devices_page';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
}));

jest.mock('@/fleet/hooks/use_devices', () => ({
  useDevices: () => ({
    data: {
      devices: [
        { id: '1', dutId: 'dut-1' },
        { id: '2', dutId: 'dut-2' },
      ],
    },
    isPending: false,
    isFetching: false,
  }),
}));

jest.mock('./use_chromeos_current_tasks', () => ({
  useChromeOSCurrentTasks: () => ({
    tasks: {},
    error: null,
    isError: false,
    isPending: false,
  }),
}));

describe('<ChromeOSDevicesPage />', () => {
  beforeAll(() => {
    window.URL.createObjectURL = jest.fn(() => 'mock-url');
    window.URL.revokeObjectURL = jest.fn();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('should render', async () => {
    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: ['/test/chromeos'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeVisible();
  });

  it('should allow clicking action buttons when rows are selected', async () => {
    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: ['/test/chromeos'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Wait for data to load and rows to appear
    const checkboxes = await screen.findAllByRole('checkbox');
    expect(checkboxes.length).toBeGreaterThan(1);

    // Click the first row checkbox (index 1, index 0 is select all)
    fireEvent.click(checkboxes[1]);

    // Check that "Run autorepair" button is visible
    const autorepairButton = screen.getByRole('button', {
      name: /run autorepair/i,
    });
    expect(autorepairButton).toBeVisible();

    // Click it
    fireEvent.click(autorepairButton);
  });
  it('should allow customizing columns', async () => {
    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: ['/test/chromeos'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Find and click "Columns" button
    const columnsButton = await screen.findByText('Columns');
    expect(columnsButton).toBeVisible();
    fireEvent.click(columnsButton);

    // Wait for the dropdown to appear
    const searchInput = screen.getByPlaceholderText(/search/i);
    expect(searchInput).toBeVisible();

    // Verify that there are checkboxes in the dropdown
    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes.length).toBeGreaterThan(1);
  });

  describe('with Reserve DUTs feature flag', () => {
    afterEach(() => {
      localStorage.clear();
    });

    it('shows reserve button in toolbar when enabled and row is selected', async () => {
      localStorage.setItem('featureFlag:fleet-console:reserve-duts', 'on');
      render(
        <FakeContextProvider
          mountedPath="/test/:platform"
          routerOptions={{
            initialEntries: ['/test/chromeos'],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <ChromeOSDevicesPage />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>,
      );

      const checkboxes = await screen.findAllByRole('checkbox');
      fireEvent.click(checkboxes[1]);

      const reserveButton = await screen.findByRole('button', {
        name: /reserve/i,
      });
      expect(reserveButton).toBeVisible();
    });

    it('hides reserve button in toolbar when disabled and row is selected', async () => {
      localStorage.setItem('featureFlag:fleet-console:reserve-duts', 'off');
      render(
        <FakeContextProvider
          mountedPath="/test/:platform"
          routerOptions={{
            initialEntries: ['/test/chromeos'],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <ChromeOSDevicesPage />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>,
      );

      const checkboxes = await screen.findAllByRole('checkbox');
      fireEvent.click(checkboxes[1]);

      const autorepairButton = await screen.findByRole('button', {
        name: /run autorepair/i,
      });
      expect(autorepairButton).toBeVisible();

      const reserveButton = screen.queryByRole('button', {
        name: /reserve/i,
      });
      expect(reserveButton).toBeNull();
    });
  });

  it('should call ExportDevicesToCSV with correct filters', async () => {
    const mockExport = jest.fn().mockReturnValue({
      queryKey: ['ExportDevicesToCSV'],
      queryFn: jest.fn().mockResolvedValue({ csvData: 'id,dut_id\n1,dut-1\n' }),
      data: { csvData: 'id,dut_id\n1,dut-1\n' },
      isPending: false,
    });

    const mockGetDimensions = jest.fn().mockReturnValue({
      queryKey: ['GetDeviceDimensions'],
      queryFn: jest.fn().mockResolvedValue({
        baseDimensions: {},
        labels: {
          'label-pool': { values: ['cellular'] },
          ufs_zone: { values: ['ZONE_CHROMEOS7'] },
        },
      }),
      data: {
        baseDimensions: {},
        labels: {
          'label-pool': { values: ['cellular'] },
          ufs_zone: { values: ['ZONE_CHROMEOS7'] },
        },
      },
      isPending: false,
    });

    const mockCountDevices = jest.fn().mockReturnValue({
      queryKey: ['CountDevices'],
      queryFn: jest.fn().mockResolvedValue({ totalHosts: 0, offlineHosts: 0 }),
      data: { totalHosts: 0, offlineHosts: 0 },
      isPending: false,
    });

    jest.spyOn(PrpcClients, 'useFleetConsoleClient').mockReturnValue({
      ExportDevicesToCSV: {
        query: mockExport,
      },
      GetDeviceDimensions: {
        query: mockGetDimensions,
      },
      CountDevices: {
        query: mockCountDevices,
      },
      CountBrowserDevices: {
        query: mockCountDevices,
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);

    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: [
            '/test/chromeos?filters=' +
              encodeURIComponent(
                '(labels."label-pool" = "cellular") AND (labels."ufs_zone" = "ZONE_CHROMEOS7")',
              ),
          ],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Wait for filter chips to parse and load from GetDeviceDimensions query
    await screen.findAllByTestId('filter-chip');

    // Find and click the Export button
    const exportButton = await screen.findByRole('button', { name: /export/i });
    fireEvent.click(exportButton);

    // Find and click "Export all (CSV)" menu item
    const exportAllItem = await screen.findByText('Export all (CSV)');
    await act(async () => {
      fireEvent.click(exportAllItem);
    });

    // Verify that the query was triggered with parsed and formatted filter keys
    expect(mockExport).toHaveBeenCalled();
    const callArgs = mockExport.mock.lastCall![0];
    expect(callArgs.filter).toBe(
      '(labels."label-pool" = "cellular") AND (labels."ufs_zone" = "ZONE_CHROMEOS7")',
    );
  });
});
