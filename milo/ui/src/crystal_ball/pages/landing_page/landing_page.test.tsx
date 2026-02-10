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

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { LandingPage } from '@/crystal_ball/pages/landing_page';

describe('<LandingPage />', () => {
  it('should render the landing page', () => {
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>,
    );
    expect(
      screen.getByRole('heading', { name: /CrystalBall Dashboards/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /Demo Page/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/Search dashboards.../i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /New Dashboard/i }),
    ).toBeInTheDocument();
  });

  test('shows toast when clicking New Dashboard', async () => {
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>,
    );

    const newDashboardButton = screen.getByRole('button', {
      name: /New Dashboard/i,
    });
    newDashboardButton.click();

    expect(
      await screen.findByText(/Feature under construction/i),
    ).toBeVisible();
  });

  test('shows toast when clicking a dashboard row', async () => {
    render(
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>,
    );

    const dashboardName = await screen.findByText(/Generic Dashboard Alpha/i);
    dashboardName.click();

    expect(
      await screen.findByText(/Feature under construction/i),
    ).toBeVisible();
  });
});
