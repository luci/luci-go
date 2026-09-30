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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { platformRoutes } from './platform_routes';

jest.mock('@/fleet/pages/chromeos/health', () => ({
  __esModule: true,
  default: () => (
    <div data-testid="chromeos-health-page">ChromeOS Health Page</div>
  ),
}));

jest.mock('@/fleet/pages/chromeos/repairs', () => ({
  __esModule: true,
  default: () => (
    <div data-testid="chromeos-repairs-page">ChromeOS Repairs Page</div>
  ),
}));

jest.mock('@/fleet/pages/chromeos/repairs/workforce_activity_view', () => ({
  __esModule: true,
  default: () => (
    <div data-testid="chromeos-workforce-page">ChromeOS Workforce Page</div>
  ),
}));

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

  describe('health route', () => {
    it('renders ChromeOS health page for ChromeOS platform', async () => {
      render(
        <FakeContextProvider
          siblingRoutes={[
            {
              path: 'p/:platform',
              children: platformRoutes,
            },
          ]}
          routerOptions={{
            initialEntries: ['/p/chromeos/health'],
          }}
        >
          <></>
        </FakeContextProvider>,
      );

      expect(
        await screen.findByTestId('chromeos-health-page'),
      ).toBeInTheDocument();
      expect(screen.queryByText('Page not found')).not.toBeInTheDocument();
    });

    it('renders PlatformNotAvailable on unsupported platforms', async () => {
      render(
        <FakeContextProvider
          siblingRoutes={[
            {
              path: 'p/:platform',
              children: platformRoutes,
            },
          ]}
          routerOptions={{
            initialEntries: ['/p/android/health'],
          }}
        >
          <></>
        </FakeContextProvider>,
      );

      expect(
        await screen.findByText('Platform not available'),
      ).toBeInTheDocument();
      expect(
        screen.queryByTestId('chromeos-health-page'),
      ).not.toBeInTheDocument();
    });
  });

  describe('repairs and workforce routes', () => {
    beforeEach(() => {
      localStorage.clear();
      sessionStorage.clear();
    });

    it('renders ChromeOS repairs page when repairs flag is enabled', async () => {
      localStorage.setItem(
        'featureFlag:fleet-console:chromeos-repairs-dashboard',
        'on',
      );

      render(
        <FakeContextProvider
          siblingRoutes={[
            {
              path: 'p/:platform',
              children: platformRoutes,
            },
          ]}
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs'],
          }}
        >
          <></>
        </FakeContextProvider>,
      );

      expect(
        await screen.findByTestId('chromeos-repairs-page'),
      ).toBeInTheDocument();
    });

    it('renders ChromeOS workforce page when both repairs and workforce flags are enabled', async () => {
      localStorage.setItem(
        'featureFlag:fleet-console:chromeos-repairs-dashboard',
        'on',
      );
      localStorage.setItem(
        'featureFlag:fleet-console:chromeos-workforce-activity',
        'on',
      );

      render(
        <FakeContextProvider
          siblingRoutes={[
            {
              path: 'p/:platform',
              children: platformRoutes,
            },
          ]}
          routerOptions={{
            initialEntries: ['/p/chromeos/repairs/workforce'],
          }}
        >
          <></>
        </FakeContextProvider>,
      );

      expect(
        await screen.findByTestId('chromeos-workforce-page'),
      ).toBeInTheDocument();
    });
  });
});
