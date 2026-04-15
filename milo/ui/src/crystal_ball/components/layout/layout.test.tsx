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

import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import {
  CRYSTAL_BALL_BASE_PATH,
  CRYSTAL_BALL_ROUTES,
} from '@/crystal_ball/routes';
import { QueuedStickyScrollingBase } from '@/generic_libs/components/queued_sticky';

import { Layout } from './layout';

jest.mock('@/common/components/auth_state_provider', () => ({
  useAuthState: () => ({
    identity: 'test-identity',
  }),
}));

describe('Layout', () => {
  const renderWithStickyContext = (ui: React.ReactNode) => {
    return render(<QueuedStickyScrollingBase>{ui}</QueuedStickyScrollingBase>);
  };

  it('renders TopBar and Outlet content', () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={['/crystal-ball']}>
        <Routes>
          <Route path="/crystal-ball" element={<Layout />}>
            <Route index element={<div>Content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByTitle('Home')).toBeInTheDocument();
  });

  it('displays "CrystalBall Dashboards" on landing page', () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={[CRYSTAL_BALL_ROUTES.LANDING]}>
        <Routes>
          <Route path={CRYSTAL_BALL_BASE_PATH} element={<Layout />}>
            <Route index element={<div>Landing Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const title = screen.getByText(COMMON_MESSAGES.CRYSTAL_BALL_DASHBOARDS);
    expect(title).toBeInTheDocument();
  });

  it('displays home icon on other pages and shows tooltip on hover', async () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={[`${CRYSTAL_BALL_BASE_PATH}/other`]}>
        <Routes>
          <Route path={CRYSTAL_BALL_BASE_PATH} element={<Layout />}>
            <Route path="other" element={<div>Other Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const logo = screen.getByTitle('Home');
    fireEvent.mouseOver(logo);

    expect(
      await screen.findByRole('tooltip', { name: 'CrystalBall Dashboards' }),
    ).toBeInTheDocument();
  });
});
