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

import { QueuedStickyScrollingBase } from '@/generic_libs/components/queued_sticky';

import { Layout } from './layout';

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
  });

  it('displays "CBD" on landing page', () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={['/ui/labs/crystal-ball']}>
        <Routes>
          <Route path="/ui/labs/crystal-ball" element={<Layout />}>
            <Route index element={<div>Landing Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const title = screen.getByText('CBD');
    expect(title).toBeInTheDocument();
  });

  it('displays "CBD" on other pages and shows tooltip on hover', async () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={['/ui/labs/crystal-ball/demo']}>
        <Routes>
          <Route path="/ui/labs/crystal-ball" element={<Layout />}>
            <Route path="demo" element={<div>Demo Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const logo = screen.getByText('CBD');
    fireEvent.mouseOver(logo);

    expect(
      await screen.findByRole('tooltip', { name: 'CrystalBall Dashboards' }),
    ).toBeInTheDocument();
  });

  it('opens overflow menu and shows link to Demo Page', () => {
    renderWithStickyContext(
      <MemoryRouter initialEntries={['/crystal-ball']}>
        <Routes>
          <Route path="/crystal-ball" element={<Layout />}>
            <Route index element={<div>Landing Page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const menuButton = screen.getByLabelText('show more');
    fireEvent.click(menuButton);

    const demoLink = screen.getByText('Demo Page');
    expect(demoLink).toBeInTheDocument();
    expect(demoLink.closest('a')).toHaveAttribute(
      'href',
      '/ui/labs/crystal-ball/demo',
    );
  });
});
