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

import {
  QueryClient,
  QueryClientProvider,
  UseQueryResult,
} from '@tanstack/react-query';
import { act, render, screen, fireEvent, within } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { SettingsProvider } from '@/fleet/context/providers';
import { ORDER_BY_PARAM_KEY } from '@/fleet/hooks/order_by';
import * as PrpcClients from '@/fleet/hooks/prpc_clients';
import { createMockUseFleetConsoleClient } from '@/fleet/testing_tools/mocks/fleet_console_client';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import {
  ListRepairMetricsResponse,
  RepairMetric,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RepairListPage } from './repairs_page';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

describe('RepairListPage', () => {
  let queryClient: QueryClient;
  let mockUseFleetConsoleClient: jest.SpyInstance;

  const createMockClientWithMetrics = (metrics: readonly RepairMetric[]) => {
    const response = {
      repairMetrics: metrics,
      nextPageToken: '',
    };
    return createMockUseFleetConsoleClient({
      data: response,
      queryFn: jest.fn().mockResolvedValue(response),
    } as unknown as Partial<UseQueryResult<ListRepairMetricsResponse, Error>>);
  };

  const renderComponent = (
    workspace: AndroidPageWorkspace = 'Android',
    mockClient = createMockUseFleetConsoleClient(),
  ) => {
    mockUseFleetConsoleClient.mockImplementation(mockClient);

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider>
          <SettingsProvider>
            <ShortcutProvider>
              <RepairListPage workspace={workspace} />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );
    act(() => {
      jest.runAllTimers();
    });
  };

  beforeEach(() => {
    jest.useFakeTimers();
    mockTrackEvent.mockClear();
    queryClient = new QueryClient();

    mockUseFleetConsoleClient = jest
      .spyOn(PrpcClients, 'useFleetConsoleClient')
      .mockImplementation(createMockUseFleetConsoleClient());
  });

  describe('with default android page', () => {
    beforeEach(() => {
      renderComponent('Android');
    });

    it('renders the component and calls the mock', () => {
      expect(screen.getByText('Repair metrics')).toBeInTheDocument();
      expect(screen.getByText('Offline / Total Devices')).toBeInTheDocument();
      expect(mockUseFleetConsoleClient).toHaveBeenCalled();
    });

    it('renders table data', async () => {
      expect(await screen.findByText('lab1')).toBeInTheDocument();
      expect(await screen.findByText('lab2')).toBeInTheDocument();
      expect(await screen.findByText('sjc-mdpt9-wear')).toBeInTheDocument();
    });

    it('generates the correct omnilab links', async () => {
      expect(await screen.findByText('sjc-mdpt9-wear')).toBeInTheDocument();

      const links = screen.getAllByRole('link');
      const hrefs = links.map((link) => link.getAttribute('href'));

      // Check link for lab1 (uses lab_location)
      expect(hrefs).toContain(
        'https://omnilab.corp.google.com/recovery?host=lab_location%3Ainclude%3Alab1&host=host_group%3Ainclude%3Agroup1',
      );

      // Check link for sjc-mdpt9-wear (uses lab)
      expect(hrefs).toContain(
        'https://omnilab.corp.google.com/recovery?host=lab%3Ainclude%3Asjc-mdpt9-wear&host=host_group%3Ainclude%3Agroup3',
      );
    });

    it('allows opening filter menu for Lab Name', async () => {
      // Wait for the table to be rendered and headers to be available
      await act(async () => {
        jest.runAllTimers();
      });

      const header = await screen.findByText('Lab Name');
      const th = header.closest('th');
      const filterButton = th?.querySelector('.ColumnActionsMenuButton');
      expect(filterButton).toBeTruthy();

      await act(async () => fireEvent.click(filterButton!));

      expect(screen.getByText('Filter')).toBeVisible();
    });

    it('tracks Explore in Arsenal clicks in google analytics', async () => {
      const labRow = (await screen.findByText('lab1')).closest('tr')!;
      const link = within(labRow).getByRole('link', {
        name: /explore in arsenal/i,
      });

      fireEvent.click(link);

      expect(mockTrackEvent).toHaveBeenCalledWith(
        'explore_in_arsenal_clicked',
        {
          totalDevices: 2,
        },
      );
    });

    it('tracks Explore devices clicks in google analytics', async () => {
      const labRow = (await screen.findByText('lab1')).closest('tr')!;
      const link = within(labRow).getByRole('link', {
        name: /explore devices/i,
      });

      fireEvent.click(link);

      expect(mockTrackEvent).toHaveBeenCalledWith('explore_devices_clicked', {
        totalDevices: 2,
      });
    });
  });

  describe('explore devices link', () => {
    it('works for Android workspace', async () => {
      renderComponent('Android');
      expect(await screen.findByText('lab1')).toBeInTheDocument();

      const labRow = (await screen.findByText('lab1')).closest('tr')!;
      const link = within(labRow).getByRole('link', {
        name: /explore devices/i,
      });
      const url = new URL(link.getAttribute('href')!, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/android/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "lab1") AND ("host_group" = "group1") AND ("run_target" = "target1")',
      );
    });

    it('works for Pixel workspace', async () => {
      renderComponent(
        'Pixel',
        createMockClientWithMetrics([
          {
            priority: RepairMetric_Priority.WATCH,
            labName: 'pixel-lab',
            hostGroup: 'custom-group',
            runTarget: 'pixel-target',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
        ]),
      );
      expect(await screen.findByText('pixel-lab')).toBeInTheDocument();

      const pixelRow = (await screen.findByText('pixel-lab')).closest('tr')!;
      const link = within(pixelRow).getByRole('link', {
        name: /explore devices/i,
      });
      const url = new URL(link.getAttribute('href')!, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/pixel/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "pixel-lab") AND ("host_group" = "custom-group") AND ("run_target" = "pixel-target")',
      );
    });

    it('filters out pte_labs host_group for Pixel workspace', async () => {
      renderComponent(
        'Pixel',
        createMockClientWithMetrics([
          {
            priority: RepairMetric_Priority.WATCH,
            labName: 'pixel-lab',
            hostGroup: 'pte_labs',
            runTarget: 'pixel-target',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
        ]),
      );
      expect(await screen.findByText('pixel-lab')).toBeInTheDocument();

      const pixelRow = (await screen.findByText('pixel-lab')).closest('tr')!;
      const link = within(pixelRow).getByRole('link', {
        name: /explore devices/i,
      });
      const url = new URL(link.getAttribute('href')!, 'http://localhost');
      expect(url.pathname).toBe('/ui/fleet/p/pixel/devices');
      expect(url.searchParams.get(ORDER_BY_PARAM_KEY)).toBe('state desc');
      expect(url.searchParams.get(FILTERS_PARAM_KEY)).toBe(
        '("lab_name" = "pixel-lab") AND ("run_target" = "pixel-target")',
      );
    });

    it('generates include blank in the url if a metric (lab_name, host_group, run_target) is empty', async () => {
      renderComponent(
        'Android',
        createMockClientWithMetrics([
          {
            priority: RepairMetric_Priority.WATCH,
            labName: '',
            hostGroup: 'group-only',
            runTarget: 'target-only',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
          {
            priority: RepairMetric_Priority.WATCH,
            labName: 'lab-only',
            hostGroup: '',
            runTarget: 'target-for-empty-group',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
          {
            priority: RepairMetric_Priority.WATCH,
            labName: 'lab-for-empty-target',
            hostGroup: 'group-for-empty-target',
            runTarget: '',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
          {
            priority: RepairMetric_Priority.WATCH,
            labName: '',
            hostGroup: '',
            runTarget: '',
            minimumRepairs: 1,
            devicesOffline: 1,
            totalDevices: 2,
            peakUsage: 1,
          },
        ]),
      );

      // Verify row with empty lab_name
      const emptyLabRow = (await screen.findByText('group-only')).closest(
        'tr',
      )!;
      const emptyLabLink = within(emptyLabRow).getByRole('link', {
        name: /explore devices/i,
      });
      const emptyLabUrl = new URL(
        emptyLabLink.getAttribute('href')!,
        'http://localhost',
      );
      const emptyLabFilters = emptyLabUrl.searchParams.get(FILTERS_PARAM_KEY);
      expect(emptyLabFilters).toBe(
        'NOT "lab_name":* AND ("host_group" = "group-only") AND ("run_target" = "target-only")',
      );
      expect(emptyLabFilters).toContain('NOT "lab_name":*');

      // Verify row with empty host_group
      const emptyGroupRow = (await screen.findByText('lab-only')).closest(
        'tr',
      )!;
      const emptyGroupLink = within(emptyGroupRow).getByRole('link', {
        name: /explore devices/i,
      });
      const emptyGroupUrl = new URL(
        emptyGroupLink.getAttribute('href')!,
        'http://localhost',
      );
      const emptyGroupFilters =
        emptyGroupUrl.searchParams.get(FILTERS_PARAM_KEY);
      expect(emptyGroupFilters).toBe(
        '("lab_name" = "lab-only") AND NOT "host_group":* AND ("run_target" = "target-for-empty-group")',
      );
      expect(emptyGroupFilters).toContain('NOT "host_group":*');

      // Verify row with empty run_target
      const emptyTargetRow = (
        await screen.findByText('lab-for-empty-target')
      ).closest('tr')!;
      const emptyTargetLink = within(emptyTargetRow).getByRole('link', {
        name: /explore devices/i,
      });
      const emptyTargetUrl = new URL(
        emptyTargetLink.getAttribute('href')!,
        'http://localhost',
      );
      const emptyTargetFilters =
        emptyTargetUrl.searchParams.get(FILTERS_PARAM_KEY);
      expect(emptyTargetFilters).toBe(
        '("lab_name" = "lab-for-empty-target") AND ("host_group" = "group-for-empty-target") AND NOT "run_target":*',
      );
      expect(emptyTargetFilters).toContain('NOT "run_target":*');

      // Verify row where all metrics (lab_name, host_group, run_target) are empty
      const allRows = screen.getAllByRole('row');
      const allEmptyRow = allRows[allRows.length - 1];
      const allEmptyLink = within(allEmptyRow).getByRole('link', {
        name: /explore devices/i,
      });
      const allEmptyUrl = new URL(
        allEmptyLink.getAttribute('href')!,
        'http://localhost',
      );
      const allEmptyFilters = allEmptyUrl.searchParams.get(FILTERS_PARAM_KEY);
      expect(allEmptyFilters).toBe(
        'NOT "lab_name":* AND NOT "host_group":* AND NOT "run_target":*',
      );
      expect(allEmptyFilters).toContain('NOT "lab_name":*');
      expect(allEmptyFilters).toContain('NOT "host_group":*');
      expect(allEmptyFilters).toContain('NOT "run_target":*');
    });
  });
});
