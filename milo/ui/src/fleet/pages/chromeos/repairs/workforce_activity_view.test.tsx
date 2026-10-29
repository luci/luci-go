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

import { AuthState } from '@/common/api/auth_state';
import { getFeatureFlagLocalStorageKey } from '@/common/feature_flags';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import {
  enableWorkforceInProgressRepairs,
  enableWorkforcePickupRank,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import * as UseClaimRepairTaskModule from '@/fleet/pages/chromeos/repairs/use_claim_repair_task';
import * as UseWorkforceActivityModule from '@/fleet/pages/chromeos/repairs/use_workforce_activity';
import { GetWorkforceActivityRequest_Timeframe } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Component, WorkforceActivityView } from './workforce_activity_view';

const mockNavigate = jest.fn();
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useNavigate: () => mockNavigate,
}));

const mockTrackEvent = jest.fn();
jest.mock('@/generic_libs/components/google_analytics', () => ({
  ...jest.requireActual('@/generic_libs/components/google_analytics'),
  useGoogleAnalytics: () => ({ trackEvent: mockTrackEvent }),
  TrackLeafRoutePageView: ({ children }: { children: React.ReactNode }) =>
    children,
}));

const MOCK_WORKFORCE_DATA = {
  activeRepairers: 4,
  totalPriorityPointsCleared: 2850,
  avgQueuePickupRank: 4.7,
  inProgressRepairs: 2,
  avgRepairTime: { seconds: '1680', nanos: 0 },
  technicians: [
    {
      id: 'andrew',
      name: 'Andrew Miller',
      email: 'andrew@google.com',
      claimedDuts: [
        {
          taskId: '201',
          dutId: 'chromeos1-row2-rack3-host1',
          score: 250,
          duration: { seconds: '2700', nanos: 0 },
        },
      ],
      priorityScoreCleared: 680,
      avgPtsPerDut: 85,
      avgQueuePickupRank: 1.8,
      isRankSkewed: false,
      completedRepairs: 8,
      avgDuration: { seconds: '1920', nanos: 0 },
    },
    {
      id: 'beatrice',
      name: 'Beatrice Chen',
      email: 'beatrice@google.com',
      claimedDuts: [
        {
          taskId: '202',
          dutId: 'chromeos2-row3-rack1-host2',
          score: 85,
          duration: { seconds: '1800', nanos: 0 },
        },
      ],
      priorityScoreCleared: 940,
      avgPtsPerDut: 85,
      avgQueuePickupRank: 1.2,
      isRankSkewed: true,
      completedRepairs: 11,
      avgDuration: { seconds: '1440', nanos: 0 },
    },
    {
      id: 'charlie',
      name: '',
      email: 'user:charlie@google.com',
      claimedDuts: [],
      priorityScoreCleared: 0,
      avgPtsPerDut: 0,
      avgQueuePickupRank: 0,
      isRankSkewed: false,
      completedRepairs: 0,
      avgDuration: undefined,
    },
  ],
};

const enableAllMetricFlags = () => {
  localStorage.setItem(
    getFeatureFlagLocalStorageKey(enableWorkforcePickupRank),
    'on',
  );
  localStorage.setItem(
    getFeatureFlagLocalStorageKey(enableWorkforceInProgressRepairs),
    'on',
  );
  localStorage.setItem(
    getFeatureFlagLocalStorageKey(enableWorkforceTimeframeFilter),
    'on',
  );
};

describe('<WorkforceActivityView />', () => {
  let queryClient: QueryClient;

  let mockUnclaimMutate: jest.Mock;

  beforeEach(() => {
    localStorage.clear();
    mockNavigate.mockReset();
    mockUnclaimMutate = jest.fn();
    jest
      .spyOn(UseClaimRepairTaskModule, 'useUnclaimRepairTask')
      .mockReturnValue({
        mutate: mockUnclaimMutate,
        isPending: false,
      } as unknown as ReturnType<
        typeof UseClaimRepairTaskModule.useUnclaimRepairTask
      >);
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

  const renderWorkforceView = (props = {}, authState?: AuthState) =>
    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider
          mountedPath="/p/:platform/repairs/workforce"
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs/workforce'],
          }}
        >
          <FakeAuthStateProvider value={authState}>
            <SettingsProvider>
              <ShortcutProvider>
                <WorkforceActivityView {...props} />
              </ShortcutProvider>
            </SettingsProvider>
          </FakeAuthStateProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

  it('renders header, info banner, and KPI summary cards when metric feature flags are enabled', () => {
    enableAllMetricFlags();
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(
      screen.getByText('Workforce & Technician Monitoring'),
    ).toBeInTheDocument();
    expect(screen.getByText('Active Repairers')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(
      screen.getByText('Total Priority Points Cleared'),
    ).toBeInTheDocument();
    expect(screen.getByText('2,850 pts')).toBeInTheDocument();
    expect(screen.getByText('Avg Queue Pickup Rank')).toBeInTheDocument();
    expect(screen.getByText('#4.7')).toBeInTheDocument();
    expect(screen.getByText('In-Progress Repairs')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('Avg Repair Time (MTTR)')).toBeInTheDocument();
  });

  it('renders technician table rows with details, avatars, and metrics when metric feature flags are enabled', () => {
    enableAllMetricFlags();
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(screen.getByText('Andrew Miller')).toBeInTheDocument();
    expect(screen.getByText('andrew@google.com')).toBeInTheDocument();
    expect(screen.getByText('680 pts')).toBeInTheDocument();
    expect(screen.getByText('#1.8')).toBeInTheDocument();
    expect(screen.getByText('8 devices')).toBeInTheDocument();

    expect(screen.getByText('Beatrice Chen')).toBeInTheDocument();
    expect(screen.getByText('beatrice@google.com')).toBeInTheDocument();
    expect(screen.getByText('940 pts')).toBeInTheDocument();
    expect(screen.getByText('#1.2')).toBeInTheDocument();
    expect(screen.getByText('11 devices')).toBeInTheDocument();
    expect(screen.queryByText(/Sum:/i)).not.toBeInTheDocument();

    expect(screen.getByText('No active devices claimed')).toBeInTheDocument();
  });

  it('calls unclaimTask with taskId for any technician row when in-progress flag is enabled', () => {
    localStorage.setItem(
      getFeatureFlagLocalStorageKey(enableWorkforceInProgressRepairs),
      'on',
    );
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView(
      {},
      {
        identity: 'user:Andrew@Google.com',
        email: 'Andrew@Google.com',
        accessToken: '',
        idToken: '',
      },
    );

    expect(screen.getByText(/chromeos1-row2-rack3-host1/i)).toBeInTheDocument();
    expect(screen.getByText(/chromeos2-row3-rack1-host2/i)).toBeInTheDocument();

    // Both Andrew's and Beatrice's rows should render delete icons.
    const deleteIcons = screen.getAllByTestId('CancelIcon');
    expect(deleteIcons).toHaveLength(2);

    fireEvent.click(deleteIcons[0]);
    expect(mockUnclaimMutate).toHaveBeenCalledWith({
      taskId: '201',
      etag: '',
    });

    fireEvent.click(deleteIcons[1]);
    expect(mockUnclaimMutate).toHaveBeenCalledWith({
      taskId: '202',
      etag: '',
    });
  });

  it('shows at most two highest time units in claimed DUT chips', () => {
    localStorage.setItem(
      getFeatureFlagLocalStorageKey(enableWorkforceInProgressRepairs),
      'on',
    );
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: {
          ...MOCK_WORKFORCE_DATA,
          technicians: [
            {
              ...MOCK_WORKFORCE_DATA.technicians[0],
              claimedDuts: [
                {
                  taskId: '301',
                  dutId: 'dut-multi-units',
                  score: 120,
                  // 1 day, 2 hr, 15 min, 30 sec = 86400 + 7200 + 900 + 30 = 94530
                  duration: { seconds: '94530', nanos: 0 },
                },
                {
                  taskId: '302',
                  dutId: 'dut-three-units',
                  score: 90,
                  // 1 hr, 2 min, 30 sec = 3750
                  duration: { seconds: '3750', nanos: 0 },
                },
              ],
            },
          ],
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(
      screen.getByText('dut-multi-units (Score: 120 | 1 day, 2 hr)'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('dut-three-units (Score: 90 | 1 hr, 2 min)'),
    ).toBeInTheDocument();
  });

  it('shows at most 2 claimed DUTs initially, expands all on "And x more...", and collapses on "Show less"', () => {
    localStorage.setItem(
      getFeatureFlagLocalStorageKey(enableWorkforceInProgressRepairs),
      'on',
    );
    const manyDuts = Array.from({ length: 4 }, (_, i) => ({
      taskId: `40${i + 1}`,
      dutId: `chromeos-dut-${i + 1}`,
      score: 100,
      duration: { seconds: '1800', nanos: 0 },
    }));

    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: {
          ...MOCK_WORKFORCE_DATA,
          technicians: [
            {
              ...MOCK_WORKFORCE_DATA.technicians[0],
              claimedDuts: manyDuts,
            },
          ],
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(screen.getByText(/chromeos-dut-1/i)).toBeInTheDocument();
    expect(screen.getByText(/chromeos-dut-2/i)).toBeInTheDocument();
    expect(screen.queryByText(/chromeos-dut-3/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/chromeos-dut-4/i)).not.toBeInTheDocument();

    const moreButton = screen.getByRole('button', { name: 'And 2 more...' });
    expect(moreButton).toBeInTheDocument();

    fireEvent.click(moreButton);

    expect(screen.getByText(/chromeos-dut-3/i)).toBeInTheDocument();
    expect(screen.getByText(/chromeos-dut-4/i)).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /And \d+ more\.\.\./i }),
    ).not.toBeInTheDocument();

    const showLessButton = screen.getByRole('button', { name: 'Show less' });
    expect(showLessButton).toBeInTheDocument();

    fireEvent.click(showLessButton);

    expect(screen.queryByText(/chromeos-dut-3/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/chromeos-dut-4/i)).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'And 2 more...' }),
    ).toBeInTheDocument();
  });

  it('disables claimed DUT chips while unclaim is pending and omits onDelete when taskId is empty or 0', () => {
    localStorage.setItem(
      getFeatureFlagLocalStorageKey(enableWorkforceInProgressRepairs),
      'on',
    );
    jest
      .spyOn(UseClaimRepairTaskModule, 'useUnclaimRepairTask')
      .mockReturnValue({
        mutate: mockUnclaimMutate,
        isPending: true,
      } as unknown as ReturnType<
        typeof UseClaimRepairTaskModule.useUnclaimRepairTask
      >);
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: {
          ...MOCK_WORKFORCE_DATA,
          technicians: [
            {
              ...MOCK_WORKFORCE_DATA.technicians[0],
              claimedDuts: [
                {
                  taskId: '0',
                  dutId: 'dut-zero-task',
                  score: 100,
                  duration: { seconds: '600', nanos: 0 },
                },
                {
                  taskId: '',
                  dutId: 'dut-empty-task',
                  score: 100,
                  duration: { seconds: '600', nanos: 0 },
                },
              ],
            },
          ],
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView(
      {},
      {
        identity: 'user:andrew@google.com',
        email: 'andrew@google.com',
        accessToken: '',
        idToken: '',
      },
    );

    expect(screen.queryByTestId('CancelIcon')).not.toBeInTheDocument();
    const chip = screen.getByText(/dut-zero-task/i).closest('.MuiChip-root');
    expect(chip).toHaveClass('Mui-disabled');
  });

  it('changes timeframe when toggle buttons are clicked', () => {
    localStorage.setItem(
      getFeatureFlagLocalStorageKey(enableWorkforceTimeframeFilter),
      'on',
    );
    const useWorkforceSpy = jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(useWorkforceSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        timeframe: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_1D,
      }),
    );

    fireEvent.click(
      screen.getByRole('button', {
        name: /Last Week \(7D\)/i,
      }),
    );
    expect(useWorkforceSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        timeframe: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_7D,
      }),
    );

    fireEvent.click(
      screen.getByRole('button', {
        name: /Last Month \(30D\)/i,
      }),
    );
    expect(useWorkforceSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        timeframe: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_30D,
      }),
    );

    fireEvent.click(
      screen.getByRole('button', {
        name: /^YTD$/i,
      }),
    );
    expect(useWorkforceSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        timeframe: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_YTD,
      }),
    );
  });

  it('navigates back to the repair queue on back button click', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    const backButton = screen.getByRole('button', {
      name: /Prioritized Manual Repair Queue/i,
    });
    fireEvent.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith(
      '/ui/fleet/p/chromeos/repairs?lab=MTV',
    );
  });

  it('calls onBackToQueue prop when supplied on back button click', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    const mockBack = jest.fn();
    renderWorkforceView({ onBackToQueue: mockBack });

    const backButton = screen.getByRole('button', {
      name: /Prioritized Manual Repair Queue/i,
    });
    fireEvent.click(backButton);

    expect(mockBack).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('renders skeleton placeholders in KPI cards and spinner in table when loading', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: undefined,
        isPending: true,
        isError: false,
        isFetching: true,
        isLoading: true,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    const skeletons = screen.getAllByTestId('kpi-skeleton');
    expect(skeletons).toHaveLength(5);
    expect(screen.getByTestId('workforce-loading-spinner')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(screen.queryByText('Andrew Miller')).not.toBeInTheDocument();
  });

  it('renders error alert and error table row when RPC fails with isError', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: undefined,
        isPending: false,
        isError: true,
        error: new Error('Permission denied to fetch workforce data'),
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(screen.getByTestId('workforce-activity-error')).toBeInTheDocument();
    expect(
      screen.getByText(/Permission denied to fetch workforce data/i),
    ).toBeInTheDocument();
    expect(screen.getByTestId('workforce-error-table')).toBeInTheDocument();
  });

  it('renders empty table message when no technicians are returned', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: {
          activeRepairers: 0,
          totalPriorityPointsCleared: 0,
          avgQueuePickupRank: 0,
          inProgressRepairs: 0,
          avgRepairTime: undefined,
          technicians: [],
        },
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    renderWorkforceView();

    expect(screen.getByTestId('workforce-empty-table')).toBeInTheDocument();
    expect(
      screen.getByText(
        /No workforce activity recorded for the selected timeframe \(1D\)\./i,
      ),
    ).toBeInTheDocument();
  });

  it('renders page Component wrapper when feature flag is enabled', () => {
    jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        isFetching: false,
        isLoading: false,
        isPlaceholderData: false,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider
          mountedPath="/p/:platform/repairs/workforce"
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs/workforce'],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <Component />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText('Workforce & Technician Monitoring'),
    ).toBeInTheDocument();
  });

  it('renders PageNotFoundPage in Component wrapper when feature flag is disabled', () => {
    localStorage.setItem(
      'featureFlag:fleet-console:chromeos-workforce-activity',
      'off',
    );

    render(
      <QueryClientProvider client={queryClient}>
        <FakeContextProvider
          mountedPath="/p/:platform/repairs/workforce"
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs/workforce'],
          }}
        >
          <SettingsProvider>
            <ShortcutProvider>
              <Component />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByText('Page not found')).toBeInTheDocument();
  });

  it('passes lab to useWorkforceActivity', () => {
    const spy = jest
      .spyOn(UseWorkforceActivityModule, 'useWorkforceActivity')
      .mockReturnValue({
        data: MOCK_WORKFORCE_DATA,
        isPending: false,
        isError: false,
        error: null,
      } as unknown as ReturnType<
        typeof UseWorkforceActivityModule.useWorkforceActivity
      >);

    render(
      <FakeContextProvider>
        <WorkforceActivityView lab="EM25" />
      </FakeContextProvider>,
    );

    expect(spy).toHaveBeenCalledWith(
      expect.objectContaining({
        lab: 'EM25',
      }),
    );
    expect(screen.getByTestId('repair-lab-select')).toHaveTextContent(
      'Lab: EM25',
    );
  });
});
