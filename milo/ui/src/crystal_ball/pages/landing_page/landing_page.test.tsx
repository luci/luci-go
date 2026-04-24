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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { TopBar } from '@/crystal_ball/components/layout/top_bar';
import { TopBarProvider } from '@/crystal_ball/components/layout/top_bar_provider';
import {
  useCreateDashboardWorkflow,
  useDeleteDashboardState,
  useGenerateDashboardWorkflow,
  useListDashboardStatesInfinite,
  useSuggestMeasurementFilterValues,
  useUndeleteDashboardState,
} from '@/crystal_ball/hooks';
import { LandingPage } from '@/crystal_ball/pages/landing_page';
import { CRYSTAL_BALL_ROUTES } from '@/crystal_ball/routes';
import {
  createMockInfiniteQueryResult,
  createMockMutationResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import { DashboardState } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

jest.mock('@/crystal_ball/hooks', () => ({
  useCreateDashboardState: jest.fn(),
  useCreateDashboardWorkflow: jest.fn(),
  useDeleteDashboardState: jest.fn(),
  useGenerateDashboardWorkflow: jest.fn(),
  useListDashboardStatesInfinite: jest.fn(),
  useSuggestMeasurementFilterValues: jest.fn(),
  useUndeleteDashboardState: jest.fn(),
}));

const mockNavigate = jest.fn();
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useNavigate: () => mockNavigate,
}));

const mockDashboards: DashboardState[] = [
  DashboardState.fromPartial({
    name: 'dashboardStates/dashboard1',
    dashboardContent: {
      widgets: [],
      dataSpecs: {},
      globalFilters: [],
    },
    displayName: 'Generic Dashboard Alpha',
    description: 'A mock dashboard description',
    updateTime: '2025-01-01T00:00:00Z',
    createTime: '2025-01-01T00:00:00Z',
    revisionId: 'rev1',
    etag: 'etag1',
    uid: 'uid1',
    reconciling: false,
  }),
];

describe('<LandingPage />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.mocked(useListDashboardStatesInfinite).mockReturnValue(
      createMockInfiniteQueryResult({
        pages: [
          {
            dashboardStates: mockDashboards,
            nextPageToken: '',
            totalSize: mockDashboards.length,
          },
        ],
        pageParams: [null],
      }),
    );
    jest.mocked(useCreateDashboardWorkflow).mockReturnValue({
      createDashboard: jest.fn(),
      isPending: false,
      errorMsg: '',
      setErrorMsg: jest.fn(),
    });
    jest.mocked(useGenerateDashboardWorkflow).mockReturnValue({
      generateDashboard: jest.fn(),
      isPending: false,
      errorMsg: '',
      setErrorMsg: jest.fn(),
    });
    jest.mocked(useSuggestMeasurementFilterValues).mockReturnValue(
      createMockQueryResult({
        values: [],
        suggestions: [],
      }),
    );
    jest.mocked(useDeleteDashboardState).mockReturnValue(
      createMockMutationResult({
        mutateAsync: jest.fn(),
      }),
    );
    jest.mocked(useUndeleteDashboardState).mockReturnValue(
      createMockMutationResult({
        mutateAsync: jest.fn(),
      }),
    );
  });

  it('should render the landing page', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    expect(
      await screen.findByRole('link', { name: /CrystalBall Dashboards/i }),
    ).toBeInTheDocument();

    expect(
      screen.getByRole('button', { name: /New Dashboard/i }),
    ).toBeInTheDocument();
  });

  test('opens Create Dashboard modal when clicking New Dashboard', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    const newDashboardButton = screen.getByRole('button', {
      name: /New Dashboard/i,
    });
    fireEvent.click(newDashboardButton);

    expect(
      await screen.findByRole('dialog', { name: /Create New Dashboard/i }),
    ).toBeVisible();
  });

  test('opens Generate Dashboard modal when clicking Generate Dashboard', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    const generateButton = screen.getByRole('button', {
      name: /Generate Dashboard/i,
    });
    fireEvent.click(generateButton);

    expect(
      await screen.findByRole('dialog', { name: /Generate Dashboard/i }),
    ).toBeVisible();
  });

  test('navigates to dashboard page when clicking a dashboard row', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    const dashboardName = await screen.findByText(/Generic Dashboard Alpha/i);
    fireEvent.click(dashboardName);

    expect(mockNavigate).toHaveBeenCalledWith(
      CRYSTAL_BALL_ROUTES.DASHBOARD_DETAIL('dashboard1'),
    );
  });

  test('submits create dashboard form', async () => {
    const mockCreateDashboard = jest.fn(async (_data, onSuccess) => {
      onSuccess?.();
      mockNavigate(CRYSTAL_BALL_ROUTES.DASHBOARD_DETAIL('newly-created-id'));
    });
    jest.mocked(useCreateDashboardWorkflow).mockReturnValue({
      createDashboard: mockCreateDashboard,
      isPending: false,
      errorMsg: '',
      setErrorMsg: jest.fn(),
    });

    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: /New Dashboard/i }));

    const dialog = await screen.findByRole('dialog', {
      name: /Create New Dashboard/i,
    });
    expect(dialog).toBeVisible();

    const nameInput = screen.getByLabelText(/Dashboard Name/i);
    fireEvent.change(nameInput, { target: { value: 'New Test Dashboard' } });

    const descInput = screen.getByLabelText(/Description/i);
    fireEvent.change(descInput, { target: { value: 'Description here' } });

    fireEvent.click(screen.getByRole('button', { name: /Save/i }));

    await waitFor(() => {
      expect(mockCreateDashboard).toHaveBeenCalledWith(
        {
          displayName: 'New Test Dashboard',
          description: 'Description here',
        },
        expect.any(Function),
      );
    });

    // Verify modal closed and navigate called
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith(
        CRYSTAL_BALL_ROUTES.DASHBOARD_DETAIL('newly-created-id'),
      );
    });
  });

  test('submits generate dashboard form', async () => {
    const mockGenerateDashboard = jest.fn(async (_data, onSuccess) => {
      onSuccess?.();
    });
    jest.mocked(useGenerateDashboardWorkflow).mockReturnValue({
      generateDashboard: mockGenerateDashboard,
      isPending: false,
      errorMsg: '',
      setErrorMsg: jest.fn(),
    });

    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    fireEvent.click(
      screen.getByRole('button', { name: /Generate Dashboard/i }),
    );

    const dialog = await screen.findByRole('dialog', {
      name: /Generate Dashboard/i,
    });
    expect(dialog).toBeVisible();

    const promptInput = screen.getByLabelText(
      /What kind of dashboard do you want?/i,
    );
    fireEvent.change(promptInput, { target: { value: 'Cold startup times' } });

    fireEvent.click(screen.getByRole('button', { name: /Generate/i }));

    await waitFor(() => {
      expect(mockGenerateDashboard).toHaveBeenCalledWith(
        {
          prompt: 'Cold startup times',
          metricKeys: [],
        },
        expect.any(Function),
      );
    });
  });

  test('switches to Deleted Dashboards tab and loads deleted dashboards', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [CRYSTAL_BALL_ROUTES.LANDING],
        }}
        mountedPath={CRYSTAL_BALL_ROUTES.LANDING}
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    // Initial render should call with showDeleted: false (or undefined)
    expect(useListDashboardStatesInfinite).toHaveBeenCalledWith(
      expect.not.objectContaining({ showDeleted: true }),
    );

    // Click the Deleted Dashboards tab
    const deletedTab = screen.getByRole('tab', { name: /Deleted Dashboards/i });
    fireEvent.click(deletedTab);

    // Should call useListDashboardStatesInfinite again with showDeleted: true
    await waitFor(() => {
      expect(useListDashboardStatesInfinite).toHaveBeenCalledWith(
        expect.objectContaining({ showDeleted: true }),
      );
    });
  });
});
