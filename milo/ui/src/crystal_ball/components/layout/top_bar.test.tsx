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
import { ReactNode } from 'react';
import { MemoryRouter } from 'react-router';

import { TopBar } from './top_bar';
import { useTopBarConfig } from './top_bar_context';
import { TopBarProvider } from './top_bar_provider';

// Helper component to set context values
function ConfigSetter({
  title = null,
  actions = null,
}: {
  title?: ReactNode;
  actions?: ReactNode;
}) {
  useTopBarConfig(title, actions);
  return null;
}

describe('TopBar', () => {
  const renderTopBar = (
    initialEntries = ['/ui/labs/crystal-ball'],
    title?: ReactNode,
    actions?: ReactNode,
  ) => {
    return render(
      <TopBarProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <TopBar />
          <ConfigSetter title={title} actions={actions} />
        </MemoryRouter>
      </TopBarProvider>,
    );
  };

  it('renders "CBD" on landing page', () => {
    renderTopBar(['/ui/labs/crystal-ball']);
    expect(screen.getByText('CBD')).toBeInTheDocument();
  });

  it('renders "CBD" on other pages', () => {
    renderTopBar(['/ui/labs/crystal-ball/demo']);
    expect(screen.getByText('CBD')).toBeInTheDocument();
  });

  it('shows tooltip "CrystalBall Dashboards" on hover', async () => {
    renderTopBar(['/ui/labs/crystal-ball/demo']);

    const logo = screen.getByText('CBD');
    fireEvent.mouseOver(logo);

    expect(
      await screen.findByRole('tooltip', { name: 'CrystalBall Dashboards' }),
    ).toBeInTheDocument();
  });

  it('renders page title when provided', () => {
    renderTopBar(['/ui/labs/crystal-ball'], 'My Page Title');
    expect(screen.getByText('My Page Title')).toBeInTheDocument();
  });

  it('renders actions when provided', () => {
    const actionButton = <button>My Action</button>;
    renderTopBar(['/ui/labs/crystal-ball'], undefined, actionButton);
    expect(screen.getByText('My Action')).toBeInTheDocument();
  });

  it('opens overflow menu and navigates to Demo Page', () => {
    renderTopBar();

    const menuButton = screen.getByLabelText('show more');
    fireEvent.click(menuButton);

    const demoLink = screen.getByRole('menuitem', { name: 'Demo Page' });
    expect(demoLink).toBeInTheDocument();
    expect(demoLink).toHaveAttribute('href', '/ui/labs/crystal-ball/demo');
  });

  it('renders divider when title is present', () => {
    renderTopBar(['/ui/labs/crystal-ball'], 'Title');
    const divider = screen.getByRole('separator');
    expect(divider).toBeInTheDocument();
  });
});
