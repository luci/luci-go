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

import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import * as useDashboardStateApi from '@/crystal_ball/hooks/use_dashboard_state_api';
import { DashboardPage } from '@/crystal_ball/pages/dashboard_page';
import { DashboardState } from '@/crystal_ball/types';

jest.mock('@/crystal_ball/hooks/use_dashboard_state_api', () => ({
  useGetDashboardState: jest.fn(),
  useUpdateDashboardState: jest.fn(() => ({
    mutateAsync: jest.fn(),
    isPending: false,
  })),
}));

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ dashboardId: 'abcd123' }),
}));

jest.mock('@/generic_libs/hooks/synced_search_params', () => {
  let mockParams = new URLSearchParams('');
  return {
    useSyncedSearchParams: jest.fn(() => [mockParams, jest.fn()]),
    setMockParams: (paramsString: string) => {
      mockParams = new URLSearchParams(paramsString);
    },
  };
});

jest.mock('@/crystal_ball/components/layout/top_bar_context', () => ({
  useTopBarConfig: jest.fn(),
}));

const { setMockParams } = jest.requireMock(
  '@/generic_libs/hooks/synced_search_params',
);

const mockDashboard: DashboardState = {
  name: 'dashboardStates/abcd123',
  displayName: 'Test Dashboard',
  dashboardContent: { widgets: [] },
};

describe('<DashboardPage />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setMockParams('');
  });

  const renderDashboard = () => {
    const { rerender } = render(<DashboardPage />);
    return { rerender };
  };

  it('renders loading state initially', () => {
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: true,
      error: null,
      refetch: jest.fn(),
      data: null,
    });
    renderDashboard();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('renders error state', () => {
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: new Error('Network error'),
      refetch: jest.fn(),
      data: null,
    });
    renderDashboard();
    expect(screen.getByText(/Failed to load dashboard/)).toBeInTheDocument();
  });

  it('renders not found state', () => {
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: null,
      refetch: jest.fn(),
      data: null,
    });
    renderDashboard();
    expect(screen.getByText(/Dashboard not found/)).toBeInTheDocument();
  });

  it('renders dashboard data and sets top bar config', async () => {
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: null,
      refetch: jest.fn(),
      data: mockDashboard,
    });
    renderDashboard();
    expect(await screen.findByText(/abcd123/)).toBeInTheDocument();
    expect(useTopBarConfig).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.anything(),
    );
  });

  it('shows toast when time range changes', async () => {
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: null,
      refetch: jest.fn(),
      data: mockDashboard,
    });

    setMockParams('time_option=12h');
    const { rerender } = renderDashboard();

    expect(
      screen.queryByText(/Feature under construction/i),
    ).not.toBeInTheDocument();

    setMockParams('time_option=6h');
    rerender(<DashboardPage />);

    // Toast is visible on subsequent render with new params
    await waitFor(() => {
      expect(screen.getByText(/Feature under construction/i)).toBeVisible();
    });
  });

  it('saves edits and shows success toast', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({});
    (useDashboardStateApi.useUpdateDashboardState as jest.Mock).mockReturnValue(
      {
        mutateAsync: mockMutateAsync,
        isPending: false,
      },
    );
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: null,
      refetch: jest.fn(),
      data: mockDashboard,
    });

    renderDashboard();

    const lastCall = (useTopBarConfig as jest.Mock).mock.calls.slice(-1)[0];
    const { unmount } = render(
      <>
        {lastCall[0]}
        {lastCall[1]}
      </>,
    );

    fireEvent.click(
      screen.getByRole('button', {
        name: /Edit dashboard title and description/i,
      }),
    );

    fireEvent.change(screen.getByLabelText(/Dashboard Name/i), {
      target: { value: 'Updated Name' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

    unmount();

    const newCall = (useTopBarConfig as jest.Mock).mock.calls.slice(-1)[0];
    render(
      <>
        {newCall[0]}
        {newCall[1]}
      </>,
    );

    const saveButton = screen.getByRole('button', { name: 'Save' });
    expect(saveButton).not.toBeDisabled();
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith({
        dashboardState: expect.objectContaining({
          displayName: 'Updated Name',
        }),
        updateMask: { paths: ['displayName', 'description'] },
      });
      expect(screen.getByText('Dashboard saved successfully')).toBeVisible();
    });
  });

  it('displays error toast on save failure', async () => {
    const mockMutateAsync = jest.fn().mockRejectedValue(new Error('API Error'));
    (useDashboardStateApi.useUpdateDashboardState as jest.Mock).mockReturnValue(
      {
        mutateAsync: mockMutateAsync,
        isPending: false,
      },
    );
    (useDashboardStateApi.useGetDashboardState as jest.Mock).mockReturnValue({
      isLoading: false,
      error: null,
      refetch: jest.fn(),
      data: mockDashboard,
    });

    renderDashboard();

    const lastCall = (useTopBarConfig as jest.Mock).mock.calls.slice(-1)[0];
    const { unmount } = render(
      <>
        {lastCall[0]}
        {lastCall[1]}
      </>,
    );

    fireEvent.click(
      screen.getByRole('button', {
        name: /Edit dashboard title and description/i,
      }),
    );

    fireEvent.change(screen.getByLabelText(/Dashboard Name/i), {
      target: { value: 'Updated Name' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

    unmount();

    const newCall = (useTopBarConfig as jest.Mock).mock.calls.slice(-1)[0];
    render(
      <>
        {newCall[0]}
        {newCall[1]}
      </>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(screen.getByText(/API Error/i)).toBeVisible();
    });
  });
});
