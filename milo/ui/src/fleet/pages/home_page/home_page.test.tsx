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

import { render, screen, waitFor, within } from '@testing-library/react';

import { useUserProfile } from '@/common/hooks/use_user_profile';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HomePage } from './home_page';

jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(),
}));

jest.mock('@/common/hooks/use_user_profile', () => ({
  useUserProfile: jest.fn(),
}));

describe('<HomePage />', () => {
  beforeEach(() => {
    (useUserProfile as jest.Mock).mockReturnValue({
      isAnonymous: false,
      displayFirstName: 'Test',
      email: 'test@example.com',
      picture: undefined,
    });

    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform],
          queryFn: () => {
            if (req.platform === Platform.CHROMEOS) {
              return Promise.resolve({
                total: 1000,
                deviceState: { ready: 900 },
              });
            }
            if (req.platform === Platform.ANDROID) {
              return Promise.resolve({
                androidCount: {
                  totalDevices: 500,
                  idleDevices: 250,
                  busyDevices: 100,
                },
              });
            }
            return Promise.resolve({});
          },
        })),
      },
      CountRepairMetrics: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountRepairMetrics', req.platform],
          queryFn: () => {
            if (req.platform === Platform.ANDROID) {
              return Promise.resolve({
                offlineDevices: 300,
              });
            }
            return Promise.resolve({});
          },
        })),
      },
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountBrowserDevices'],
          queryFn: () =>
            Promise.resolve({
              total: 200,
              swarmingState: { total: 200, alive: 150 },
            }),
        })),
      },
    });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  it('renders home page correctly', async () => {
    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Welcome to Fleet Console (FCon), Test!'),
    ).toBeInTheDocument();
    expect(screen.getByText('ChromeOS')).toBeInTheDocument();
    expect(screen.getByText('Browser')).toBeInTheDocument();
    expect(screen.getByText('Android')).toBeInTheDocument();

    // Check external links
    expect(screen.getByText('Learn more').closest('a')).toHaveAttribute(
      'href',
      'http://go/fleet-console',
    );
    expect(screen.getByText('File feedback').closest('a')).toHaveAttribute(
      'href',
      expect.stringContaining('https://issuetracker.google.com/issues/new'),
    );
    expect(
      screen.getByText('Subscribe to updates').closest('a'),
    ).toHaveAttribute('href', 'http://g/fleet-console-users');

    await waitFor(() => {
      const chromeosCard = screen
        .getByText('ChromeOS')
        .closest('.MuiCard-root') as HTMLElement;
      expect(
        within(chromeosCard).getByText('View all devices'),
      ).toHaveAttribute('href', '/ui/fleet/p/chromeos/devices');
      expect(
        within(chromeosCard).getByText('Total Devices').closest('a'),
      ).toHaveAttribute('href', '/ui/fleet/p/chromeos/devices');

      const browserCard = screen
        .getByText('Browser')
        .closest('.MuiCard-root') as HTMLElement;
      expect(within(browserCard).getByText('View all devices')).toHaveAttribute(
        'href',
        '/ui/fleet/p/chromium/devices',
      );
      expect(
        within(browserCard).getByText('Total Devices').closest('a'),
      ).toHaveAttribute('href', '/ui/fleet/p/chromium/devices');

      const androidCard = screen
        .getByText('Android')
        .closest('.MuiCard-root') as HTMLElement;
      expect(within(androidCard).getByText('View all devices')).toHaveAttribute(
        'href',
        '/ui/fleet/p/android/devices',
      );
      expect(
        within(androidCard).getByText('Total Devices').closest('a'),
      ).toHaveAttribute('href', '/ui/fleet/p/android/devices');
      expect(within(androidCard).getByText('View all repairs')).toHaveAttribute(
        'href',
        '/ui/fleet/p/android/repairs',
      );
      expect(
        within(androidCard).getByText('Devices offline').closest('a'),
      ).toHaveAttribute('href', '/ui/fleet/p/android/repairs');
    });
  });

  it('prompts user to log in when anonymous', async () => {
    (useUserProfile as jest.Mock).mockReturnValue({
      isAnonymous: true,
      displayFirstName: '',
      email: undefined,
      picture: undefined,
    });

    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    expect(screen.getByText(/You must/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /login/i })).toBeInTheDocument();
  });

  it('displays the loaded data and no longer calculates percentages', async () => {
    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('1,000')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('300')).toBeInTheDocument();
  });

  it('displays the error state if counts fail to load', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform],
          isError: true,
        })),
      },
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountBrowserDevices'],
          isError: true,
        })),
      },
      CountRepairMetrics: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountRepairMetrics'],
          isError: true,
        })),
      },
    });

    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    const errorMessages = await screen.findAllByText('Error loading data');
    expect(errorMessages.length).toBe(3); // One for each platform
  });
});
