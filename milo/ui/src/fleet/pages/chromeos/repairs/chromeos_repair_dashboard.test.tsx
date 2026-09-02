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
import { fireEvent, render, screen } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as UseClaimRepairTaskModule from '@/fleet/pages/chromeos/repairs/use_claim_repair_task';
import * as UseRepairQueueModule from '@/fleet/pages/chromeos/repairs/use_repair_queue';
import {
  PeripheralState,
  RepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
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
    taskId: '101',
    dutId: 'chromeos15-row2-rack3-host4',
    pools: ['DUT_POOL_QUOTA'],
    model: 'volteer',
    state: 'needs_repair',
    claimedBy: '',
    claimedAt: undefined,
    servoState: PeripheralState.PERIPHERAL_STATE_OK,
    wifiState: PeripheralState.PERIPHERAL_STATE_BROKEN,
    bluetoothState: PeripheralState.PERIPHERAL_STATE_MISSING,
  },
  {
    taskId: '102',
    dutId: 'chromeos15-row2-rack3-host5',
    pools: ['faft-cr50'],
    model: 'brya',
    state: 'repair_failed',
    claimedBy: 'tech1@google.com',
    claimedAt: '2026-08-19T10:00:00Z',
    servoState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
    wifiState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
    bluetoothState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
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

  it('renders all 6 columns: Dut ID, Pool, Model, State, Peripherals, Assignee', async () => {
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
    expect(screen.getByText('Pool')).toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('Peripherals (W / B / S)')).toBeInTheDocument();
    expect(screen.getByText('Assignee')).toBeInTheDocument();
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

    // Verify peripheral icons for both rows
    expect(screen.getByLabelText('Servo: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Wi-Fi: BROKEN')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: MISSING')).toBeInTheDocument();
    expect(screen.getAllByLabelText('Wi-Fi: N/A')).toHaveLength(1);
    expect(screen.getAllByLabelText('Bluetooth: N/A')).toHaveLength(1);
    expect(screen.getAllByLabelText('Servo: N/A')).toHaveLength(1);
  });

  it('renders Claim button for unclaimed item and Avatar for claimed item', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: [
          ...MOCK_QUEUE_ITEMS,
          {
            taskId: '103',
            dutId: 'chromeos15-row2-rack3-host6',
            pool: 'DUT_POOL_QUOTA',
            model: 'volteer',
            state: 'needs_repair',
            claimedBy: '   ',
            claimedAt: undefined,
          },
        ],
        totalSize: 3,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    // Unclaimed items (including whitespace claimedBy) should render Claim buttons
    const claimButtons = screen.getAllByRole('button', { name: /Claim/i });
    expect(claimButtons).toHaveLength(2);

    // Claimed item should render circular Avatar with initial 'T'
    const avatar = screen.getByText('T');
    expect(avatar).toBeInTheDocument();
  });

  it('invokes claim mutation when Claim button is clicked', async () => {
    const mockMutate = jest.fn();
    jest.spyOn(UseClaimRepairTaskModule, 'useClaimRepairTask').mockReturnValue({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      isSuccess: false,
    } as unknown as ReturnType<
      typeof UseClaimRepairTaskModule.useClaimRepairTask
    >);

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

    const claimButton = screen.getByRole('button', { name: /Claim/i });
    fireEvent.click(claimButton);

    expect(mockMutate).toHaveBeenCalledTimes(1);
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: '101',
      }),
    );
  });

  it('invokes unclaim mutation when own Avatar is clicked', async () => {
    const mockUnclaimMutate = jest.fn();
    jest
      .spyOn(UseClaimRepairTaskModule, 'useUnclaimRepairTask')
      .mockReturnValue({
        mutate: mockUnclaimMutate,
        isPending: false,
        isError: false,
        isSuccess: false,
      } as unknown as ReturnType<
        typeof UseClaimRepairTaskModule.useUnclaimRepairTask
      >);

    const ownClaimedItem: RepairQueueItem = {
      taskId: '103',
      dutId: 'chromeos15-row2-rack3-host7',
      pools: ['DUT_POOL_QUOTA'],
      model: 'volteer',
      state: 'needs_repair',
      claimedBy: 'user@example.com',
      claimedAt: '2026-08-19T10:00:00Z',
      servoState: PeripheralState.PERIPHERAL_STATE_OK,
      wifiState: PeripheralState.PERIPHERAL_STATE_OK,
      bluetoothState: PeripheralState.PERIPHERAL_STATE_OK,
    };

    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: [ownClaimedItem],
        totalSize: 1,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    // Default mock user is user@example.com -> Initial is 'U'
    const avatar = screen.getByText('U');
    fireEvent.click(avatar);

    expect(mockUnclaimMutate).toHaveBeenCalledTimes(1);
    expect(mockUnclaimMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: '103',
      }),
    );
  });

  it('invokes claim mutation when another user Avatar is clicked', async () => {
    const mockClaimMutate = jest.fn();
    jest.spyOn(UseClaimRepairTaskModule, 'useClaimRepairTask').mockReturnValue({
      mutate: mockClaimMutate,
      isPending: false,
      isError: false,
      isSuccess: false,
    } as unknown as ReturnType<
      typeof UseClaimRepairTaskModule.useClaimRepairTask
    >);

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

    // tech1@google.com avatar has initial 'T'
    const avatar = screen.getByText('T');
    fireEvent.click(avatar);

    expect(mockClaimMutate).toHaveBeenCalledTimes(1);
    expect(mockClaimMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: '102',
      }),
    );
  });

  it('disables Claim button and Avatar interactions when claim mutation is pending', async () => {
    const mockClaimMutate = jest.fn();
    jest.spyOn(UseClaimRepairTaskModule, 'useClaimRepairTask').mockReturnValue({
      mutate: mockClaimMutate,
      isPending: true,
      isError: false,
      isSuccess: false,
    } as unknown as ReturnType<
      typeof UseClaimRepairTaskModule.useClaimRepairTask
    >);

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

    const claimButton = screen.getAllByRole('button', { name: /Claim/i })[0];
    expect(claimButton).toBeDisabled();
    fireEvent.click(claimButton);
    expect(mockClaimMutate).not.toHaveBeenCalled();

    const avatar = screen.getByText('T');
    expect(avatar).toHaveStyle({
      cursor: 'not-allowed',
      opacity: '0.6',
      pointerEvents: 'none',
    });
    fireEvent.click(avatar);
    expect(mockClaimMutate).not.toHaveBeenCalled();
  });

  it('disables Claim button and Avatar interactions when unclaim mutation is pending', async () => {
    const mockUnclaimMutate = jest.fn();
    jest
      .spyOn(UseClaimRepairTaskModule, 'useUnclaimRepairTask')
      .mockReturnValue({
        mutate: mockUnclaimMutate,
        isPending: true,
        isError: false,
        isSuccess: false,
      } as unknown as ReturnType<
        typeof UseClaimRepairTaskModule.useUnclaimRepairTask
      >);

    const ownClaimedItem: RepairQueueItem = {
      taskId: '103',
      dutId: 'chromeos15-row2-rack3-host7',
      pools: ['DUT_POOL_QUOTA'],
      model: 'volteer',
      state: 'needs_repair',
      claimedBy: 'user@example.com',
      claimedAt: '2026-08-19T10:00:00Z',
      servoState: PeripheralState.PERIPHERAL_STATE_OK,
      wifiState: PeripheralState.PERIPHERAL_STATE_OK,
      bluetoothState: PeripheralState.PERIPHERAL_STATE_OK,
    };

    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: [...MOCK_QUEUE_ITEMS, ownClaimedItem],
        totalSize: 3,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    const claimButton = screen.getAllByRole('button', { name: /Claim/i })[0];
    expect(claimButton).toBeDisabled();
    fireEvent.click(claimButton);
    expect(mockUnclaimMutate).not.toHaveBeenCalled();

    const avatar = screen.getByText('U');
    expect(avatar).toHaveStyle({
      cursor: 'not-allowed',
      opacity: '0.6',
      pointerEvents: 'none',
    });
    fireEvent.click(avatar);
    expect(mockUnclaimMutate).not.toHaveBeenCalled();
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
