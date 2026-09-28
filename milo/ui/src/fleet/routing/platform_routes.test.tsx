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

import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';

import { platformRoutes } from './platform_routes';

describe('platformRoutes', () => {
  it('redirects /p/chromium to /devices', async () => {
    const router = createMemoryRouter(
      [
        {
          path: 'p/:platform',
          children: [
            platformRoutes[0],
            {
              path: 'devices',
              element: <div data-testid="devices-page">Devices Page</div>,
            },
          ],
        },
      ],
      {
        initialEntries: ['/p/chromium'],
      },
    );

    render(<RouterProvider router={router} />);

    expect(await screen.findByTestId('devices-page')).toBeInTheDocument();
  });

  it('redirects /p/chromeos to /devices', async () => {
    const router = createMemoryRouter(
      [
        {
          path: 'p/:platform',
          children: [
            platformRoutes[0],
            {
              path: 'devices',
              element: <div data-testid="devices-page">Devices Page</div>,
            },
          ],
        },
      ],
      {
        initialEntries: ['/p/chromeos'],
      },
    );

    render(<RouterProvider router={router} />);

    expect(await screen.findByTestId('devices-page')).toBeInTheDocument();
  });

  it('redirects /p/pixel to /devices', async () => {
    const router = createMemoryRouter(
      [
        {
          path: 'p/:platform',
          children: [
            platformRoutes[0],
            {
              path: 'devices',
              element: <div data-testid="devices-page">Devices Page</div>,
            },
          ],
        },
      ],
      {
        initialEntries: ['/p/pixel'],
      },
    );

    render(<RouterProvider router={router} />);

    expect(await screen.findByTestId('devices-page')).toBeInTheDocument();
  });

  it('redirects /p/android to /repairs', async () => {
    const router = createMemoryRouter(
      [
        {
          path: 'p/:platform',
          children: [
            platformRoutes[0],
            {
              path: 'repairs',
              element: <div data-testid="repairs-page">Repairs Page</div>,
            },
          ],
        },
      ],
      {
        initialEntries: ['/p/android'],
      },
    );

    render(<RouterProvider router={router} />);

    expect(await screen.findByTestId('repairs-page')).toBeInTheDocument();
  });
});
