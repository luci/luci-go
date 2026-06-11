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
          queryKey: ['CountDevices', req.platform, req.filter],
          queryFn: () => {
            if (req.platform === Platform.CHROMEOS) {
              return Promise.resolve({
                total: 1000,
                deviceState: {
                  ready: 800,
                  needRepair: 50,
                  repairFailed: 50,
                },
              });
            }
            if (req.platform === Platform.ANDROID) {
              if (req.filter === 'fc_is_offline = "true"') {
                return Promise.resolve({
                  androidCount: {
                    totalDevices: 300,
                  },
                });
              }
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

  it('displays the loaded data and calculates healthy percentages', async () => {
    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('1,000')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('300')).toBeInTheDocument();

    // Check healthy percentages
    expect(screen.getByText('90.0% Healthy')).toBeInTheDocument();
    expect(screen.getByText('40.0% Healthy')).toBeInTheDocument();
  });

  it('displays the error state if counts fail to load', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform, req.filter],
          isError: true,
        })),
      },
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountBrowserDevices'],
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

  it('displays permission warning tooltip when device count is 0', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform, req.filter],
          queryFn: () => {
            if (req.platform === Platform.CHROMEOS) {
              return Promise.resolve({
                total: 0,
              });
            }
            if (req.platform === Platform.ANDROID) {
              if (req.filter === 'fc_is_offline = "true"') {
                return Promise.resolve({
                  androidCount: {
                    totalDevices: 0,
                  },
                });
              }
              return Promise.resolve({
                androidCount: {
                  totalDevices: 0,
                },
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
              total: 0,
            }),
        })),
      },
    });

    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    const countElements = await screen.findAllByText('0');
    expect(countElements.length).toBe(4);

    const warningIcons = screen.getAllByTestId('InfoOutlinedIcon');
    expect(warningIcons.length).toBe(4);
  });

  it('rounds healthy percentage and applies correct color status', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform, req.filter],
          queryFn: () => {
            if (req.platform === Platform.CHROMEOS) {
              return Promise.resolve({
                total: 2000,
                deviceState: {
                  ready: 1799, // 1799 / 2000 * 100 = 89.95%
                },
              });
            }
            return Promise.resolve({});
          },
        })),
      },
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountBrowserDevices'],
          queryFn: () => Promise.resolve({ total: 0 }),
        })),
      },
    });

    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    // Should display '90.0% Healthy'
    const chipLabel = await screen.findByText('90.0% Healthy');
    expect(chipLabel).toBeInTheDocument();

    const chip = chipLabel.closest('.MuiChip-root');
    expect(chip).toBeInTheDocument();

    // Should have green background (high status) instead of yellow (warn status)
    expect(chip).toHaveStyleRule('background-color', 'hsl(137, 40%, 86%)');
  });

  it('clamps healthy percentage to a maximum of 100%', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CountDevices: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['CountDevices', req.platform, req.filter],
          queryFn: () => {
            if (req.platform === Platform.CHROMEOS) {
              return Promise.resolve({
                total: 100,
                deviceState: {
                  ready: 105,
                },
              });
            }
            return Promise.resolve({});
          },
        })),
      },
      CountBrowserDevices: {
        query: jest.fn().mockImplementation(() => ({
          queryKey: ['CountBrowserDevices'],
          queryFn: () => Promise.resolve({ total: 0 }),
        })),
      },
    });

    render(
      <FakeContextProvider>
        <HomePage />
      </FakeContextProvider>,
    );

    // Should display '100.0% Healthy'
    const chip = await screen.findByText('100.0% Healthy');
    expect(chip).toBeInTheDocument();
  });
});
