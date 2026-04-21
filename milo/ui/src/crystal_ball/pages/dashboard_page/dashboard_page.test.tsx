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
import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { ToastProvider } from '@/crystal_ball/context';
import * as useDashboardStateApi from '@/crystal_ball/hooks/use_dashboard_state_api';
import { DashboardPage } from '@/crystal_ball/pages/dashboard_page';
import { CRYSTAL_BALL_ROUTES } from '@/crystal_ball/routes';
import {
  createMockErrorResult,
  createMockMutationResult,
  createMockPendingResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import {
  DashboardState,
  DeleteDashboardStateRequest,
  ListMeasurementFilterColumnsResponse,
  MeasurementFilterColumn,
  PerfChartWidget,
  PerfXAxisConfig,
  PerfChartSeries,
  UpdateDashboardStateRequest,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

jest.mock('@/crystal_ball/hooks/use_dashboard_state_api', () => ({
  ...jest.requireActual('@/crystal_ball/hooks/use_dashboard_state_api'),
  getDashboardStateQueryKey: jest.fn(() => ['mock', 'query', 'key']),
  useGetDashboardState: jest.fn(),
  useUpdateDashboardState: jest.fn(() => ({
    mutateAsync: jest.fn(),
    isPending: false,
  })),
  useDeleteDashboardState: jest.fn(() => ({
    mutateAsync: jest.fn(),
    isPending: false,
  })),
  useCreateDashboardState: jest.fn(() => ({
    mutateAsync: jest.fn(),
    isPending: false,
  })),
}));

const mockUseListMeasurementFilterColumns = jest.fn(() => ({
  data: ListMeasurementFilterColumnsResponse.fromPartial({
    measurementFilterColumns: [
      { column: 'atp_test_name' },
      { column: 'build_branch' },
      { column: 'build_target' },
      { column: 'test_name' },
      { column: 'model' },
      { column: 'sku' },
    ].map((c) => MeasurementFilterColumn.fromPartial(c)),
  }),
  isLoading: false,
}));

jest.mock('@/crystal_ball/hooks/use_measurement_filter_api', () => ({
  useListMeasurementFilterColumns: () => mockUseListMeasurementFilterColumns(),
}));

jest.mock('@/common/components/auth_state_provider', () => ({
  ...jest.requireActual('@/common/components/auth_state_provider'),
  useAuthState: jest
    .fn()
    .mockReturnValue({ identity: 'user:test@example.com' }),
}));

const mockNavigate = jest.fn();
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ dashboardId: 'abcd123' }),
  useNavigate: () => mockNavigate,
  useBlocker: () => ({
    state: 'unblocked',
    proceed: jest.fn(),
    reset: jest.fn(),
  }),
}));

const mockRemoveQueries = jest.fn();
jest.mock('@tanstack/react-query', () => ({
  ...jest.requireActual('@tanstack/react-query'),
  useQueryClient: jest.fn(() => ({ removeQueries: mockRemoveQueries })),
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

jest.mock('@/crystal_ball/components', () => {
  const actual = jest.requireActual('@/crystal_ball/components');
  return {
    ...actual,
    WidgetContainer: jest.fn(({ children, onDuplicate }) => (
      <div>
        {children}
        {onDuplicate && <button onClick={onDuplicate}>Duplicate Widget</button>}
      </div>
    )),
    AddWidgetModal: jest.fn(() => <>AddWidgetModal Mock</>),
    ChartWidget: jest.fn(
      ({ onUpdate }: { onUpdate: (updatedChart: PerfChartWidget) => void }) => (
        <div>
          <span>ChartWidget Mock</span>
          <button
            onClick={() =>
              onUpdate(
                PerfChartWidget.fromPartial({
                  xAxis: PerfXAxisConfig.fromPartial({
                    column: 'build_id',
                    granularity: 1,
                  }),
                  series: [
                    PerfChartSeries.fromPartial({
                      aggregation: 2,
                    }),
                  ],
                }),
              )
            }
          >
            Update Widget
          </button>
        </div>
      ),
    ),
    MarkdownWidget: jest.fn(() => <>MarkdownWidget Mock</>),
    DashboardTimeRangeSelector: () => <>DashboardTimeRangeSelector Mock</>,
    FilterEditor: () => <>FilterEditor Mock</>,
    ShareDashboardDialog: jest.fn(() => <>ShareDashboardDialog Mock</>),
  };
});

const mockDashboard: DashboardState = DashboardState.fromPartial({
  name: 'dashboardStates/abcd123',
  displayName: 'Test Dashboard',
  dashboardContent: { widgets: [], dataSpecs: {}, globalFilters: [] },
});

describe('<DashboardPage />', () => {
  beforeAll(() => {
    jest
      .spyOn(self.crypto, 'randomUUID')
      .mockReturnValue('00000000-0000-0000-0000-000000000000');
  });

  beforeEach(() => {
    jest.clearAllMocks();
    setMockParams('');
    mockRemoveQueries.mockClear();
    mockNavigate.mockClear();
  });

  const renderDashboard = () => {
    const { rerender } = render(
      <ToastProvider>
        <DashboardPage />
      </ToastProvider>,
    );
    return { rerender };
  };

  it('renders loading state initially', () => {
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockPendingResult());
    renderDashboard();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('renders error state', () => {
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockErrorResult(new Error('Network error')));
    renderDashboard();
    expect(screen.getByText(/Failed to load dashboard/)).toBeInTheDocument();
  });

  it('renders not found state', () => {
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(undefined));
    renderDashboard();
    expect(screen.getByText(/Dashboard not found/)).toBeInTheDocument();
  });

  it('renders dashboard data and sets top bar config', async () => {
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(mockDashboard));
    renderDashboard();
    expect(await screen.findByText(/Add Widget/i)).toBeInTheDocument();
    expect(useTopBarConfig).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.anything(),
      expect.anything(),
      null,
    );
  });

  it('saves edits and shows success toast', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({});
    jest.mocked(useDashboardStateApi.useUpdateDashboardState).mockReturnValue(
      createMockMutationResult({
        mutateAsync: mockMutateAsync,
      }),
    );
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(mockDashboard));

    renderDashboard();

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
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

    const newCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
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
      expect(mockMutateAsync).toHaveBeenCalledWith(
        UpdateDashboardStateRequest.fromPartial({
          dashboardState: DashboardState.fromPartial({
            ...mockDashboard,
            displayName: 'Updated Name',
          }),
          updateMask: [
            'displayName',
            'description',
            'dashboardContent.widgets',
            'dashboardContent.globalFilters',
          ],
        }),
      );
      expect(screen.getByText('Dashboard saved successfully')).toBeVisible();
    });
  });

  it('saves updated widget settings (xAxis and series aggregation) to the API', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({});
    jest
      .mocked(useDashboardStateApi.useUpdateDashboardState)
      .mockReturnValue(
        createMockMutationResult({ mutateAsync: mockMutateAsync }),
      );

    const testWidget = {
      id: 'widget-1',
      displayName: 'Test Widget',
      chart: {
        chartType: 1,
        dataSpecId: 'dataspec-1',
      },
    };

    const dashboardWithWidget = DashboardState.fromPartial({
      ...mockDashboard,
      dashboardContent: {
        ...mockDashboard.dashboardContent,
        widgets: [testWidget],
        dataSpecs: mockDashboard.dashboardContent?.dataSpecs ?? {},
      },
    });

    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(dashboardWithWidget));

    renderDashboard();

    fireEvent.click(screen.getByRole('button', { name: 'Update Widget' }));

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
    render(<>{lastCall[1]}</>); // Render the Save button

    const saveButton = screen.getByRole('button', { name: 'Save' });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          dashboardState: expect.objectContaining({
            dashboardContent: expect.objectContaining({
              widgets: [
                expect.objectContaining({
                  chart: expect.objectContaining({
                    xAxis: expect.objectContaining({
                      column: 'build_id',
                      granularity: 1,
                    }),
                    series: [
                      expect.objectContaining({
                        aggregation: 2,
                      }),
                    ],
                  }),
                }),
              ],
            }),
          }),
        }),
      );
    });
  });

  it('displays error toast on save failure', async () => {
    const mockMutateAsync = jest.fn().mockRejectedValue(new Error('API Error'));
    jest
      .mocked(useDashboardStateApi.useUpdateDashboardState)
      .mockReturnValue(
        createMockMutationResult({ mutateAsync: mockMutateAsync }),
      );
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(mockDashboard));

    renderDashboard();

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
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

    // Unmount before rendering the new call to avoid conflicts
    unmount();

    const newCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
    render(
      <>
        {newCall[0]}
        {newCall[1]}
      </>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        UpdateDashboardStateRequest.fromPartial({
          dashboardState: DashboardState.fromPartial({
            ...mockDashboard,
            displayName: 'Updated Name',
          }),
          updateMask: [
            'displayName',
            'description',
            'dashboardContent.widgets',
            'dashboardContent.globalFilters',
          ],
        }),
      );
      expect(screen.getByText(/API Error/i)).toBeVisible();
    });
  });

  it('deletes the dashboard and navigates away on success', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({});
    jest.mocked(useDashboardStateApi.useDeleteDashboardState).mockReturnValue(
      createMockMutationResult({
        mutateAsync: mockMutateAsync,
      }),
    );
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(mockDashboard));

    renderDashboard();

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
    const { unmount } = render(<>{lastCall[2]}</>);

    fireEvent.click(screen.getByText('Delete Dashboard'));

    unmount();

    // The dialog should be visible now inside DashboardPage
    const confirmDeleteBtn = await screen.findByRole('button', {
      name: 'Delete',
    });
    fireEvent.click(confirmDeleteBtn);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        DeleteDashboardStateRequest.fromPartial({
          name: 'dashboardStates/abcd123',
        }),
      );
      expect(mockRemoveQueries).toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith(CRYSTAL_BALL_ROUTES.LANDING, {
        replace: true,
      });
      expect(screen.getByText('Dashboard deleted successfully')).toBeVisible();
    });
  });

  it('duplicates a widget and saves the updated list', async () => {
    const mockMutateAsync = jest.fn().mockResolvedValue({});
    jest
      .mocked(useDashboardStateApi.useUpdateDashboardState)
      .mockReturnValue(
        createMockMutationResult({ mutateAsync: mockMutateAsync }),
      );

    const testWidget = {
      id: 'widget-1',
      displayName: 'Test Widget',
      chart: { chartType: 1 },
    };

    const dashboardWithWidget = DashboardState.fromPartial({
      ...mockDashboard,
      dashboardContent: {
        ...mockDashboard.dashboardContent,
        widgets: [testWidget],
      },
    });

    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(dashboardWithWidget));

    renderDashboard();

    fireEvent.click(screen.getByRole('button', { name: 'Duplicate Widget' }));

    const lastCall = jest.mocked(useTopBarConfig).mock.lastCall;
    if (!lastCall) {
      throw new Error('useTopBarConfig was not called');
    }
    render(
      <>
        {lastCall[0]}
        {lastCall[1]}
      </>,
    );

    const saveButton = screen.getByRole('button', { name: 'Save' });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          dashboardState: expect.objectContaining({
            dashboardContent: expect.objectContaining({
              widgets: [
                expect.objectContaining({ id: 'widget-1' }),
                expect.objectContaining({
                  displayName: `Test Widget${COMMON_MESSAGES.COPY_SUFFIX}`,
                }),
              ],
            }),
          }),
        }),
      );
    });
  });

  it('duplicates dashboard successfully with custom title and opens in new tab', async () => {
    const mockMutateAsync = jest
      .fn()
      .mockResolvedValue({ response: { name: 'dashboardStates/new123' } });
    jest
      .mocked(useDashboardStateApi.useCreateDashboardState)
      .mockReturnValue(
        createMockMutationResult({ mutateAsync: mockMutateAsync }),
      );
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(mockDashboard));

    const originalWindowOpen = window.open;
    window.open = jest.fn();

    renderDashboard();

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
    render(<>{lastCall[2]}</>);

    fireEvent.click(screen.getByText('Copy Dashboard'));

    const titleInput = await screen.findByLabelText(/Dashboard Title/i);
    expect(titleInput).toHaveValue('[Copy] Test Dashboard');

    fireEvent.change(titleInput, { target: { value: 'Custom Clone' } });

    fireEvent.click(screen.getByRole('button', { name: 'Duplicate' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          dashboardState: expect.objectContaining({
            displayName: 'Custom Clone',
          }),
        }),
      );
      expect(window.open).toHaveBeenCalledWith(
        CRYSTAL_BALL_ROUTES.DASHBOARD_DETAIL('new123'),
        '_blank',
      );
    });

    window.open = originalWindowOpen;
  });

  it('prompts choices when unsaved edits are detected during duplication', async () => {
    const mockMutateAsync = jest
      .fn()
      .mockResolvedValue({ response: { name: 'dashboardStates/new123' } });
    jest
      .mocked(useDashboardStateApi.useCreateDashboardState)
      .mockReturnValue(
        createMockMutationResult({ mutateAsync: mockMutateAsync }),
      );

    const dashboardWithWidget = DashboardState.fromPartial({
      ...mockDashboard,
      dashboardContent: {
        widgets: [
          { id: 'widget-1', displayName: 'Orig', chart: { chartType: 1 } },
        ],
      },
    });
    jest
      .mocked(useDashboardStateApi.useGetDashboardState)
      .mockReturnValue(createMockQueryResult(dashboardWithWidget));

    renderDashboard();

    fireEvent.click(screen.getByRole('button', { name: 'Update Widget' }));

    const lastCall = jest.mocked(useTopBarConfig).mock.calls.slice(-1)[0];
    render(<>{lastCall[2]}</>);

    fireEvent.click(screen.getByText('Copy Dashboard'));

    expect(
      screen.getByText(
        /You have unsaved edits. How would you like to duplicate?/i,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('Duplicate with pending edits'),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('Duplicate original saved version'),
    ).toBeInTheDocument();
  });
});
