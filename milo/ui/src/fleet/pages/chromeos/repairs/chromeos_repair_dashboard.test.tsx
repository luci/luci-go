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
import { render, screen } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as UseRepairQueueModule from '@/fleet/pages/chromeos/repairs/use_repair_queue';
import { RepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSRepairDashboard } from './chromeos_repair_dashboard';

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

const MOCK_QUEUE_ITEMS: readonly RepairQueueItem[] = [
  {
    dutId: 'chromeos15-row2-rack3-host4',
    pool: 'DUT_POOL_QUOTA',
    model: 'volteer',
    state: 'needs_repair',
  },
  {
    dutId: 'chromeos15-row2-rack3-host5',
    pool: 'faft-cr50',
    model: 'brya',
    state: 'repair_failed',
  },
];

describe('<ChromeOSRepairDashboard />', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const renderDashboard = () =>
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider
          mountedPath="/p/:platform/repairs"
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs'],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <ChromeOSRepairDashboard />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

  it('renders page header and title without errors', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: MOCK_QUEUE_ITEMS,
        totalSize: 2,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    expect(
      screen.getByText('ChromeOS Manual Repair Dashboard'),
    ).toBeInTheDocument();
  });

  it('renders the 4 columns: Dut ID, Pool, Model, State', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: MOCK_QUEUE_ITEMS,
        totalSize: 2,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    expect(screen.getByText('Dut ID')).toBeInTheDocument();
    expect(screen.getByText('label-pool')).toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
  });

  it('populates device rows correctly with mock data', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: MOCK_QUEUE_ITEMS,
        totalSize: 2,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    expect(
      await screen.findByText('chromeos15-row2-rack3-host4'),
    ).toBeInTheDocument();
    expect(screen.getByText('DUT_POOL_QUOTA')).toBeInTheDocument();
    expect(screen.getByText('volteer')).toBeInTheDocument();
    expect(screen.getByText('NEEDS_REPAIR')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host5')).toBeInTheDocument();
    expect(screen.getByText('faft-cr50')).toBeInTheDocument();
    expect(screen.getByText('brya')).toBeInTheDocument();
    expect(screen.getByText('REPAIR_FAILED')).toBeInTheDocument();
  });

  it('displays empty list correctly', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: [],
        totalSize: 0,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    expect(
      await screen.findByText('No records to display'),
    ).toBeInTheDocument();
  });

  it('handles error state properly', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: undefined,
      error: new Error('Network error'),
      isPending: false,
      isError: true,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    expect(
      await screen.findByText('Error Loading Repair Queue'),
    ).toBeInTheDocument();
    expect(screen.getByText(/Network error/i)).toBeInTheDocument();
  });
});
