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
import * as UseWorkforceActivityModule from '@/fleet/pages/chromeos/repairs/use_workforce_activity';
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
          dutId: 'chromeos1-row2-rack3-host1',
          score: 250,
          duration: { seconds: '2700', nanos: 0 },
        },
      ],
      priorityScoreCleared: 680,
      avgPtsPerDut: 85,
      avgQueuePickupRank: 1.8,
      pickupRankSum: 14,
      pickupRankLabel: 'Top Priority',
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
          dutId: 'chromeos2-row3-rack1-host2',
          score: 85,
          duration: '1800s',
        },
      ],
      priorityScoreCleared: 940,
      avgPtsPerDut: 85,
      avgQueuePickupRank: 1.2,
      pickupRankSum: 13,
      pickupRankLabel: 'Top Priority',
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
      pickupRankSum: 0,
      pickupRankLabel: 'None',
      isRankSkewed: false,
      completedRepairs: 0,
      avgDuration: undefined,
    },
  ],
};

describe('<WorkforceActivityView />', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    mockNavigate.mockReset();
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

  const renderWorkforceView = (props = {}) =>
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
              <WorkforceActivityView {...props} />
            </ShortcutProvider>
          </SettingsProvider>
        </FakeContextProvider>
      </QueryClientProvider>,
    );

  it('renders header, info banner, and KPI summary cards correctly', () => {
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
    expect(
      screen.getByText(/Workforce roster is dynamically derived/i),
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

  it('renders technician table rows with details, avatars, and metrics', () => {
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

    expect(screen.getByText('No active devices claimed')).toBeInTheDocument();
  });

  it('allows unclaiming a claimed DUT from the technician row', () => {
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

    expect(screen.getByText(/chromeos1-row2-rack3-host1/i)).toBeInTheDocument();

    const deleteIcons = screen.getAllByTestId('CancelIcon');
    fireEvent.click(deleteIcons[0]);

    expect(
      screen.queryByText(/chromeos1-row2-rack3-host1/i),
    ).not.toBeInTheDocument();
  });

  it('changes timeframe when toggle buttons are clicked', () => {
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

    const weekButton = screen.getByRole('button', {
      name: /Last Week \(7D\)/i,
    });
    fireEvent.click(weekButton);

    expect(useWorkforceSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        timeframe: '7D',
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

    expect(mockNavigate).toHaveBeenCalledWith('/ui/fleet/p/chromeos/repairs');
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
});
