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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { DashboardListTable } from '@/crystal_ball/components/dashboard_list_table/dashboard_list_table';
import * as hooks from '@/crystal_ball/hooks';
import { DashboardState } from '@/crystal_ball/types';

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useListDashboardStatesInfinite: jest.fn(),
}));

const mockDashboards: DashboardState[] = [
  {
    name: 'dashboardStates/dashboard1',
    dashboardContent: { widgets: [] },
    displayName: 'Generic Dashboard Alpha',
    description: 'A mock dashboard description',
    updateTime: { seconds: 1735689600, nanos: 0 },
    createTime: { seconds: 1735689600, nanos: 0 },
    revisionId: 'rev1',
    etag: 'etag1',
    uid: 'uid1',
    reconciling: false,
  },
  {
    name: 'dashboardStates/dashboard2',
    dashboardContent: { widgets: [] },
    displayName: 'Generic Dashboard Beta',
    description: 'Another mock dashboard description',
    updateTime: { seconds: 1736689600, nanos: 0 },
    createTime: { seconds: 1736689600, nanos: 0 },
    revisionId: 'rev2',
    etag: 'etag2',
    uid: 'uid2',
    reconciling: false,
  },
];

describe('<DashboardListTable />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should render the table with data from useListDashboardStates', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: { pages: [{ dashboardStates: mockDashboards }] },
      isLoading: false,
      isError: false,
      isFetching: false,
    });

    render(<DashboardListTable />);

    expect(screen.getByText('Generic Dashboard Alpha')).toBeInTheDocument();
    expect(
      screen.getByText('A mock dashboard description'),
    ).toBeInTheDocument();
  });

  it('should call onDashboardClick when a row is clicked', () => {
    const onDashboardClick = jest.fn();

    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: { pages: [{ dashboardStates: mockDashboards }] },
      isLoading: false,
      isError: false,
      isFetching: false,
    });

    render(<DashboardListTable onDashboardClick={onDashboardClick} />);

    const dashboardRow = screen.getByText('Generic Dashboard Beta');
    dashboardRow.click();

    expect(onDashboardClick).toHaveBeenCalledWith(mockDashboards[1]);
  });

  it('should update filter param when searching', async () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: { pages: [{ dashboardStates: mockDashboards }] },
      isLoading: false,
      isError: false,
      isFetching: false,
    });

    render(<DashboardListTable />);

    const searchInput = screen.getByPlaceholderText('Search dashboards...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    await waitFor(() => {
      expect(hooks.useListDashboardStatesInfinite).toHaveBeenLastCalledWith(
        expect.objectContaining({
          filter: 'Alpha',
        }),
      );
    });
  });

  it('should request next page with token when clicking Load More', async () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: {
        pages: [{ dashboardStates: mockDashboards }],
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      hasNextPage: true,
      fetchNextPage: jest.fn(),
    });

    render(<DashboardListTable />);

    // Verify initial call has empty token
    await waitFor(() => {
      expect(hooks.useListDashboardStatesInfinite).toHaveBeenCalledWith(
        expect.objectContaining({
          pageSize: 20,
        }),
      );
    });

    const loadMoreButton = screen.getByRole('button', {
      name: /Load More/i,
    });

    fireEvent.click(loadMoreButton);

    expect(
      (hooks.useListDashboardStatesInfinite as jest.Mock).mock.results[0].value
        .fetchNextPage,
    ).toHaveBeenCalled();
  });

  it('should not render Load More button if there is no next page', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: {
        pages: [{ dashboardStates: mockDashboards }],
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    expect(
      screen.queryByRole('button', { name: /Load More/i }),
    ).not.toBeInTheDocument();
  });

  it('should render loading state when isLoading is true and dashboards is empty', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      isFetching: true,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    // MRT renders progress bars / skeletons when isLoading is true
    expect(screen.getAllByRole('progressbar').length).toBeGreaterThan(0);
  });

  it('should render error message in table body when isError is true', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Cannot reach API'),
      isFetching: false,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    expect(screen.getByText('Cannot reach API')).toBeInTheDocument();
  });

  it('should render parsed error message when error is a JSON string', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: {
        error: {
          code: 401,
          message: 'Request is missing required authentication credential.',
          status: 'UNAUTHENTICATED',
        },
      },
      isFetching: false,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    expect(
      screen.getByText(
        'Request is missing required authentication credential.',
      ),
    ).toBeInTheDocument();
  });

  it('should fallback to name.split if displayName is not provided', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: {
        pages: [
          {
            dashboardStates: [
              {
                name: 'dashboardStates/fallback-dash',
                dashboardContent: { widgets: [] },
                description: 'No display name here',
                updateTime: { seconds: 1735689600, nanos: 0 },
                createTime: { seconds: 1735689600, nanos: 0 },
                revisionId: 'rev1',
                etag: 'etag1',
                uid: 'uid1',
                reconciling: false,
              },
            ],
          },
        ],
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    expect(screen.getByText('fallback-dash')).toBeInTheDocument();
  });

  it('should render Unknown or Invalid date for malformed updateTime', () => {
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: {
        pages: [
          {
            dashboardStates: [
              {
                name: 'dashboardStates/invalid-dates',
                dashboardContent: { widgets: [] },
                displayName: 'Invalid Dates Dash',
                updateTime: 'this-is-not-a-date' as unknown as {
                  seconds: number;
                  nanos: number;
                },
                createTime: { seconds: 1735689600, nanos: 0 },
                revisionId: 'rev1',
                etag: 'etag1',
                uid: 'uid1',
                reconciling: false,
              },
              {
                name: 'dashboardStates/missing-dates',
                dashboardContent: { widgets: [] },
                displayName: 'Missing Dates Dash',
                updateTime: undefined as unknown as {
                  seconds: number;
                  nanos: number;
                },
                createTime: { seconds: 1735689600, nanos: 0 },
                revisionId: 'rev2',
                etag: 'etag2',
                uid: 'uid2',
                reconciling: false,
              },
            ],
          },
        ],
      },
      isLoading: false,
      isError: false,
      isFetching: false,
      hasNextPage: false,
    });

    render(<DashboardListTable />);

    expect(screen.getByText('Invalid Dates Dash')).toBeInTheDocument();
    expect(screen.getByText('Missing Dates Dash')).toBeInTheDocument();
    expect(screen.getByText('Invalid date')).toBeInTheDocument();
    expect(screen.getByText('Unknown')).toBeInTheDocument();
  });
});
