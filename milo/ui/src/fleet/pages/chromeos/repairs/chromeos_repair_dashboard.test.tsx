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

import { GrpcError } from '@chopsui/prpc-client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import * as UsePagerModule from '@/fleet/hooks/use_pager';
import * as UseClaimRepairTaskModule from '@/fleet/pages/chromeos/repairs/use_claim_repair_task';
import * as UsePriorityRulesModule from '@/fleet/pages/chromeos/repairs/use_priority_rules';
import * as UseRepairQueueModule from '@/fleet/pages/chromeos/repairs/use_repair_queue';
import {
  PeripheralState,
  RepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSRepairDashboard } from './chromeos_repair_dashboard';
import { useRepairQueueColumns } from './use_repair_queue_columns';

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
    poolHealthPct: 0.824,
    modelHealthPct: 0.95,
    priorityScore: '650',
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
    poolHealthPct: 0.45,
    modelHealthPct: 0.7,
    priorityScore: '500',
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

    jest.spyOn(UsePriorityRulesModule, 'usePriorityRules').mockReturnValue({
      rules: [],
      isLoading: false,
      isError: false,
      error: null,
      refetch: jest.fn(),
      createRule: jest.fn(),
      isCreating: false,
      createError: null,
      updateRule: jest.fn(),
      isUpdating: false,
      updateError: null,
      deleteRule: jest.fn(),
      isDeleting: false,
      deleteError: null,
    } as unknown as ReturnType<typeof UsePriorityRulesModule.usePriorityRules>);
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

    expect(
      screen.getByText('ChromeOS Manual Repair Dashboard'),
    ).toBeInTheDocument();
  });

  it('renders all 9 columns: Rank, Dut ID, Pool, Model, Pool / Model Health, State, Priority Score, Peripherals, Assignee', async () => {
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: MOCK_QUEUE_ITEMS,
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

    expect(screen.getByText('Rank')).toBeInTheDocument();
    expect(screen.getByText('Dut ID')).toBeInTheDocument();
    expect(screen.getByText('Pool')).toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('Pool / Model Health')).toBeInTheDocument();
    expect(screen.getAllByTestId('InfoOutlinedIcon')).toHaveLength(3);
    expect(screen.getByText('State')).toBeInTheDocument();
    expect(screen.getByText('Priority Score')).toBeInTheDocument();
    expect(screen.getByText('Peripherals (W / B / S)')).toBeInTheDocument();
    expect(screen.getByText('Assignee')).toBeInTheDocument();
  });

  it('populates device rows and health metrics correctly with mock data including rank and priority score', async () => {
    const items: readonly RepairQueueItem[] = [
      ...MOCK_QUEUE_ITEMS,
      {
        taskId: '103',
        dutId: 'chromeos15-row2-rack3-host6',
        pools: ['faft-cr50'],
        model: 'brya',
        state: 'repair_failed',
        servoState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        wifiState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        bluetoothState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        modelHealthPct: 0.95,
        priorityScore: '400',
      },
      {
        taskId: '104',
        dutId: 'chromeos15-row2-rack3-host7',
        pools: ['DUT_POOL_QUOTA'],
        model: 'volteer',
        state: 'needs_repair',
        servoState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        wifiState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        bluetoothState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        poolHealthPct: 0.85,
        priorityScore: '300',
      },
      {
        taskId: '105',
        dutId: 'chromeos15-row2-rack3-host8',
        pools: ['DUT_POOL_QUOTA'],
        model: 'volteer',
        state: 'needs_repair',
        servoState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        wifiState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        bluetoothState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        priorityScore: '200',
      },
      {
        taskId: '106',
        dutId: 'chromeos15-row2-rack3-host9',
        pools: ['DUT_POOL_QUOTA'],
        model: 'volteer',
        state: 'needs_repair',
        servoState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        wifiState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        bluetoothState: PeripheralState.PERIPHERAL_STATE_NOT_APPLICABLE,
        poolHealthPct: 0.0,
        modelHealthPct: 0.0,
        priorityScore: '100',
      },
    ];
    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: items,
        totalSize: 6,
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
    expect(screen.getAllByText('DUT_POOL_QUOTA').length).toBeGreaterThanOrEqual(
      1,
    );
    expect(screen.getAllByText('volteer').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('82% / 95%')).toBeInTheDocument();
    expect(screen.getAllByText('NEEDS_REPAIR').length).toBeGreaterThanOrEqual(
      1,
    );

    const row1 = screen.getByText('chromeos15-row2-rack3-host4').closest('tr');
    expect(row1).not.toBeNull();
    expect(within(row1!).getByText('1')).toBeInTheDocument();
    expect(within(row1!).getByText('650')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host5')).toBeInTheDocument();
    expect(screen.getAllByText('faft-cr50').length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText('brya').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('45% / 70%')).toBeInTheDocument();
    expect(screen.getAllByText('REPAIR_FAILED').length).toBeGreaterThanOrEqual(
      1,
    );

    // Verify peripheral icons for both rows
    expect(screen.getByLabelText('Servo: OK')).toBeInTheDocument();
    expect(screen.getByLabelText('Wi-Fi: BROKEN')).toBeInTheDocument();
    expect(screen.getByLabelText('Bluetooth: MISSING')).toBeInTheDocument();
    expect(screen.getAllByLabelText('Wi-Fi: N/A')).toHaveLength(5);
    expect(screen.getAllByLabelText('Bluetooth: N/A')).toHaveLength(5);
    expect(screen.getAllByLabelText('Servo: N/A')).toHaveLength(5);

    const row2 = screen.getByText('chromeos15-row2-rack3-host5').closest('tr');
    expect(row2).not.toBeNull();
    expect(within(row2!).getByText('2')).toBeInTheDocument();
    expect(within(row2!).getByText('500')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host6')).toBeInTheDocument();
    expect(screen.getByText('- / 95%')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host7')).toBeInTheDocument();
    expect(screen.getByText('85% / -')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host8')).toBeInTheDocument();
    expect(screen.getByText('- / -')).toBeInTheDocument();

    expect(screen.getByText('chromeos15-row2-rack3-host9')).toBeInTheDocument();
    expect(screen.getByText('0% / 0%')).toBeInTheDocument();
  });

  it('displays the priority score info tooltip with exact explanation on hover', async () => {
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

    const priorityScoreHeader = screen
      .getByText('Priority Score')
      .closest('.fleet-column-header');
    expect(priorityScoreHeader).toBeInstanceOf(HTMLElement);
    if (!(priorityScoreHeader instanceof HTMLElement)) {
      throw new Error('priorityScoreHeader must be an HTMLElement');
    }

    const infoIcon =
      within(priorityScoreHeader).getByTestId('InfoOutlinedIcon');
    expect(infoIcon).toBeInTheDocument();

    const tooltipText =
      'Devices are ranked in real time by summing active rule weights. Higher score = higher priority.';

    expect(screen.queryByText(tooltipText)).not.toBeInTheDocument();

    fireEvent.mouseOver(infoIcon);

    expect(await screen.findByText(tooltipText)).toBeInTheDocument();
  });

  it('renders dynamic 1-based ranks (1, 2, 3, 4) and defaults priority score to "0" when missing or empty', async () => {
    const fallbackItems: readonly (Omit<RepairQueueItem, 'priorityScore'> & {
      priorityScore?: string;
    })[] = [
      {
        dutId: 'fallback-dut-1',
        pools: ['default'],
        model: 'volteer',
        state: 'needs_repair',
        taskId: 'task-fallback-1',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: '0',
      },
      {
        dutId: 'fallback-dut-2',
        pools: ['default'],
        model: 'brya',
        state: 'repair_failed',
        taskId: 'task-fallback-2',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: '100',
      },
      {
        dutId: 'fallback-dut-3',
        pools: ['default'],
        model: 'corsola',
        state: 'needs_repair',
        taskId: 'task-fallback-3',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: '',
      },
      {
        dutId: 'fallback-dut-4',
        pools: ['default'],
        model: 'dedede',
        state: 'repair_failed',
        taskId: 'task-fallback-4',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: undefined,
      },
    ];

    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: fallbackItems as readonly RepairQueueItem[],
        totalSize: 4,
        nextPageToken: '',
      },
      isPending: false,
      isError: false,
      isFetching: false,
      isLoading: false,
      isPlaceholderData: false,
    } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

    renderDashboard();

    const row1 = (await screen.findByText('fallback-dut-1')).closest('tr');
    expect(row1).not.toBeNull();
    expect(within(row1!).getByText('1')).toBeInTheDocument();
    expect(within(row1!).getByText('0')).toBeInTheDocument();

    const row2 = (await screen.findByText('fallback-dut-2')).closest('tr');
    expect(row2).not.toBeNull();
    expect(within(row2!).getByText('2')).toBeInTheDocument();
    expect(within(row2!).getByText('100')).toBeInTheDocument();

    const row3 = (await screen.findByText('fallback-dut-3')).closest('tr');
    expect(row3).not.toBeNull();
    expect(within(row3!).getByText('3')).toBeInTheDocument();
    expect(within(row3!).getByText('0')).toBeInTheDocument();

    const row4 = (await screen.findByText('fallback-dut-4')).closest('tr');
    expect(row4).not.toBeNull();
    expect(within(row4!).getByText('4')).toBeInTheDocument();
    expect(within(row4!).getByText('0')).toBeInTheDocument();
  });

  it('falls back to "0" when priorityScore is empty string or undefined', async () => {
    const scoreItems: readonly (Omit<RepairQueueItem, 'priorityScore'> & {
      priorityScore?: string;
    })[] = [
      {
        dutId: 'score-empty-dut',
        pools: ['default'],
        model: 'volteer',
        state: 'needs_repair',
        taskId: 'task-score-1',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: '',
      },
      {
        dutId: 'score-undefined-dut',
        pools: ['default'],
        model: 'brya',
        state: 'repair_failed',
        taskId: 'task-score-2',
        servoState: 0,
        wifiState: 0,
        bluetoothState: 0,
        priorityScore: undefined,
      },
    ];

    jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
      data: {
        repairQueueItems: scoreItems as readonly RepairQueueItem[],
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

    const rowEmpty = (await screen.findByText('score-empty-dut')).closest('tr');
    expect(rowEmpty).not.toBeNull();
    expect(within(rowEmpty!).getByText('0')).toBeInTheDocument();

    const rowUndefined = (
      await screen.findByText('score-undefined-dut')
    ).closest('tr');
    expect(rowUndefined).not.toBeNull();
    expect(within(rowUndefined!).getByText('0')).toBeInTheDocument();
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
      priorityScore: '0',
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
      priorityScore: '0',
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

  it('renders priority scoring rules inline in the dashboard', async () => {
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
      screen.getByRole('heading', {
        level: 6,
        name: /Priority Scoring Rules/i,
      }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('add-priority-rule-button')).toBeInTheDocument();
  });

  describe('Adversarial Coverage: multi-page ranks, extreme scores, loading, errors, selection', () => {
    it('displays Page 2 global ranks (e.g. 51, 52) when pageIndex = 1 and pageSize = 50', async () => {
      jest.spyOn(UsePagerModule, 'usePager').mockReturnValue({
        pageSize: 50,
        pageToken: 'token-page-2',
        pageIndex: 1,
        goToNextPage: jest.fn(),
        goToPrevPage: jest.fn(),
        onRowsPerPageChange: jest.fn(),
      });

      const page2Items: readonly RepairQueueItem[] = [
        {
          dutId: 'chromeos15-p2-item1',
          pools: ['quota'],
          model: 'volteer',
          state: 'needs_repair',
          taskId: 'task-51',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: '120',
        },
        {
          dutId: 'chromeos15-p2-item2',
          pools: ['faft'],
          model: 'brya',
          state: 'repair_failed',
          taskId: 'task-52',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: '90',
        },
      ];

      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: {
          repairQueueItems: page2Items,
          totalSize: 150,
          nextPageToken: 'token-page-3',
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

      renderDashboard();

      expect(
        await screen.findByText('chromeos15-p2-item1'),
      ).toBeInTheDocument();
      const row1 = screen.getByText('chromeos15-p2-item1').closest('tr');
      expect(row1).not.toBeNull();
      expect(within(row1!).getByText('51')).toBeInTheDocument();
      expect(within(row1!).getByText('120')).toBeInTheDocument();

      const row2 = screen.getByText('chromeos15-p2-item2').closest('tr');
      expect(row2).not.toBeNull();
      expect(within(row2!).getByText('52')).toBeInTheDocument();
      expect(within(row2!).getByText('90')).toBeInTheDocument();
    });

    it('cleanly renders negative priority scores, MaxInt64, MinInt64, and zero scores', async () => {
      const maxInt64 = '9223372036854775807';
      const minInt64 = '-9223372036854775808';

      const extremeItems: readonly RepairQueueItem[] = [
        {
          dutId: 'dut-extreme-max',
          pools: [],
          model: 'atlas',
          state: 'needs_repair',
          taskId: 'task-max',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: maxInt64,
        },
        {
          dutId: 'dut-extreme-min',
          pools: [],
          model: 'brya',
          state: 'repair_failed',
          taskId: 'task-min',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: minInt64,
        },
        {
          dutId: 'dut-negative-score',
          pools: [],
          model: 'corsola',
          state: 'needs_repair',
          taskId: 'task-neg',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: '-75',
        },
        {
          dutId: 'dut-zero-score',
          pools: [],
          model: 'dedede',
          state: 'needs_repair',
          taskId: 'task-zero',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: '0',
        },
      ];

      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: {
          repairQueueItems: extremeItems,
          totalSize: 4,
          nextPageToken: '',
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

      renderDashboard();

      const rowMax = (await screen.findByText('dut-extreme-max')).closest('tr');
      expect(rowMax).not.toBeNull();
      expect(within(rowMax!).getByText(maxInt64)).toBeInTheDocument();
      expect(within(rowMax!).getByText('1')).toBeInTheDocument();

      const rowMin = (await screen.findByText('dut-extreme-min')).closest('tr');
      expect(rowMin).not.toBeNull();
      expect(within(rowMin!).getByText(minInt64)).toBeInTheDocument();
      expect(within(rowMin!).getByText('2')).toBeInTheDocument();

      const rowNeg = (await screen.findByText('dut-negative-score')).closest(
        'tr',
      );
      expect(rowNeg).not.toBeNull();
      expect(within(rowNeg!).getByText('-75')).toBeInTheDocument();
      expect(within(rowNeg!).getByText('3')).toBeInTheDocument();

      const rowZero = (await screen.findByText('dut-zero-score')).closest('tr');
      expect(rowZero).not.toBeNull();
      expect(within(rowZero!).getByText('0')).toBeInTheDocument();
      expect(within(rowZero!).getByText('4')).toBeInTheDocument();
    });

    it('renders progress/loading indicators gracefully without unhandled exceptions', async () => {
      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: undefined,
        isPending: true,
        isError: false,
        isFetching: true,
        isLoading: true,
        isPlaceholderData: false,
      } as unknown as ReturnType<typeof UseRepairQueueModule.useRepairQueue>);

      renderDashboard();

      expect(
        screen.getByText('ChromeOS Manual Repair Dashboard'),
      ).toBeInTheDocument();
      expect(screen.getAllByRole('progressbar').length).toBeGreaterThanOrEqual(
        1,
      );
    });

    it('handles GrpcError with INVALID_ARGUMENT (code 3)', async () => {
      const grpcErr = new GrpcError(
        3,
        'invalid AIP-160 filter expression: parse error at position 5',
      );
      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: undefined,
        error: grpcErr,
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
      expect(
        screen.getByText(
          /The information provided for fetch repair queue is invalid/i,
        ),
      ).toBeInTheDocument();
    });

    it('handles GrpcError with PERMISSION_DENIED (code 7)', async () => {
      const grpcErr = new GrpcError(
        7,
        'caller lacks fleetconsole.repairs.list permission',
      );
      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: undefined,
        error: grpcErr,
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
      expect(
        screen.getByText(/You don't have permission to fetch repair queue/i),
      ).toBeInTheDocument();
    });

    it('handles GrpcError with UNAVAILABLE (code 14)', async () => {
      const grpcErr = new GrpcError(14, 'upstream database connection timeout');
      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: undefined,
        error: grpcErr,
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
      expect(
        screen.getByText(
          /The service for fetch repair queue is currently unavailable/i,
        ),
      ).toBeInTheDocument();
    });

    it('handles non-Error primitive rejection (unknown error fallback)', async () => {
      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: undefined,
        error: 'unexpected string rejection',
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
      expect(
        screen.getByText('An unknown error occurred. Please try again.'),
      ).toBeInTheDocument();
    });

    it('verifies row selection checkboxes precede Rank column and are interactive', async () => {
      const queueItems: readonly RepairQueueItem[] = [
        {
          dutId: 'dut-select-1',
          pools: ['quota'],
          model: 'volteer',
          state: 'needs_repair',
          taskId: 'task-sel-1',
          servoState: 0,
          wifiState: 0,
          bluetoothState: 0,
          priorityScore: '100',
        },
      ];

      jest.spyOn(UseRepairQueueModule, 'useRepairQueue').mockReturnValue({
        data: {
          repairQueueItems: queueItems,
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

      const row = (await screen.findByText('dut-select-1')).closest('tr');
      expect(row).not.toBeNull();

      // Check for row checkbox
      const checkbox = within(row!).getByRole('checkbox');
      expect(checkbox).toBeInTheDocument();
      expect(checkbox).not.toBeChecked();

      // Verify clicking the checkbox toggles selection state
      fireEvent.click(checkbox);
      expect(checkbox).toBeChecked();

      // Verify cells in row: column 0 = checkbox, column 1 = Rank, column 2 = Dut ID
      const cells = within(row!).getAllByRole('cell');
      expect(cells.length).toBeGreaterThanOrEqual(3);
      // Cell 0 contains checkbox
      expect(within(cells[0]).getByRole('checkbox')).toBeInTheDocument();
      // Cell 1 contains Rank '1'
      expect(within(cells[1]).getByText('1')).toBeInTheDocument();
      // Cell 2 contains Dut ID 'dut-select-1'
      expect(within(cells[2]).getByText('dut-select-1')).toBeInTheDocument();
    });

    it('disables sorting on all repair queue table columns', () => {
      const { result } = renderHook(() => useRepairQueueColumns(), {
        wrapper: ({ children }: { children: React.ReactNode }) => (
          <FakeContextProvider>
            <QueryClientProvider client={queryClient}>
              {children}
            </QueryClientProvider>
          </FakeContextProvider>
        ),
      });

      expect(result.current.columns.length).toBeGreaterThan(0);
      for (const col of result.current.columns) {
        expect(col.enableSorting).toBe(false);
      }
    });
  });
});
