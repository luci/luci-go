// Copyright 2023 The LUCI Authors.
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
import { Outlet, RouterProvider, createMemoryRouter } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { LoginPage } from './login_page';

describe('LoginPage', () => {
  it('should have the correct login link', () => {
    const router = createMemoryRouter(
      [
        {
          element: (
            <SyncedSearchParamsProvider>
              <Outlet />
            </SyncedSearchParamsProvider>
          ),
          children: [
            {
              path: 'path/prefix',
              children: [
                {
                  index: true,
                  element: <LoginPage />,
                },
              ],
            },
          ],
        },
      ],
      { initialEntries: ['/path/prefix'] },
    );

    render(
      <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
        <RouterProvider router={router} />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByText('login')).toHaveAttribute(
      'href',
      '/auth/openid/login?r=%2F',
    );
  });

  it('should have the correct login link when redirect link is specified', () => {
    const router = createMemoryRouter(
      [
        {
          element: (
            <SyncedSearchParamsProvider>
              <Outlet />
            </SyncedSearchParamsProvider>
          ),
          children: [
            {
              path: 'path/prefix',
              children: [
                {
                  index: true,
                  element: <LoginPage />,
                },
              ],
            },
          ],
        },
      ],
      { initialEntries: ['/path/prefix?redirect=%2fto%2fhere'] },
    );
    render(
      <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
        <RouterProvider router={router} />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByText('login')).toHaveAttribute(
      'href',
      '/auth/openid/login?r=%2Fto%2Fhere',
    );
  });

  it('should redirect correctly when user signs in without clicking the login link', () => {
    const router = createMemoryRouter(
      [
        {
          element: (
            <SyncedSearchParamsProvider>
              <Outlet />
            </SyncedSearchParamsProvider>
          ),
          children: [
            {
              path: 'path/prefix',
              children: [{ index: true, element: <LoginPage /> }],
            },
            {
              path: 'to/here',
              children: [{ index: true, element: <>reached</> }],
            },
          ],
        },
      ],
      { initialEntries: ['/path/prefix?redirect=%2fto%2fhere'] },
    );
    const { rerender } = render(
      <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
        <RouterProvider router={router} />
      </FakeAuthStateProvider>,
    );
    expect(screen.queryByText('reached')).not.toBeInTheDocument();

    rerender(
      <FakeAuthStateProvider value={{ identity: 'signed-in-in-another-page' }}>
        <RouterProvider router={router} />
      </FakeAuthStateProvider>,
    );
    expect(screen.getByText('reached')).toBeInTheDocument();
  });
});
