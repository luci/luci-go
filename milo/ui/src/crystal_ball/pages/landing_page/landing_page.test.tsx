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

import { fireEvent, render, screen } from '@testing-library/react';

import { TopBar } from '@/crystal_ball/components/layout/top_bar';
import { TopBarProvider } from '@/crystal_ball/components/layout/top_bar_provider';
import * as hooks from '@/crystal_ball/hooks';
import { LandingPage } from '@/crystal_ball/pages/landing_page';
import { DashboardState } from '@/crystal_ball/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

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
    updateTime: {
      seconds: 1735689600,
      nanos: 0,
    },
    createTime: {
      seconds: 1735689600,
      nanos: 0,
    },
    revisionId: 'rev1',
    etag: 'etag1',
    uid: 'uid1',
    reconciling: false,
  },
];

describe('<LandingPage />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (hooks.useListDashboardStatesInfinite as jest.Mock).mockReturnValue({
      data: { pages: [{ dashboardStates: mockDashboards }] },
      isLoading: false,
      isError: false,
      isFetching: false,
      hasNextPage: false,
      fetchNextPage: jest.fn(),
    });
  });

  it('should render the landing page', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/ui/labs/crystal-ball'],
        }}
        mountedPath="/ui/labs/crystal-ball"
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

  test('shows toast when clicking New Dashboard', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/ui/labs/crystal-ball'],
        }}
        mountedPath="/ui/labs/crystal-ball"
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
      await screen.findByText(/Feature under construction/i),
    ).toBeVisible();
  });

  test('shows toast when clicking a dashboard row', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/ui/labs/crystal-ball'],
        }}
        mountedPath="/ui/labs/crystal-ball"
      >
        <TopBarProvider>
          <TopBar />
          <LandingPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    const dashboardName = await screen.findByText(/Generic Dashboard Alpha/i);
    fireEvent.click(dashboardName);

    expect(
      await screen.findByText(/Feature under construction/i),
    ).toBeVisible();
  });
});
