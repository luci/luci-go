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

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidSummaryHeader } from './android_summary_header';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

describe('AndroidSummaryHeader (Switcher)', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
  });

  it('renders AndroidLegacySummaryHeader when feature flag is disabled', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              totalHosts: 10,
              idleDevices: 50,
              busyDevices: 30,
              labRunningHosts: 8,
              labMissingHosts: 2,
            },
          }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(await screen.findByText('Online')).toBeInTheDocument();
    expect(screen.getByText('Offline')).toBeInTheDocument();
  });

  it('renders AndroidHealthSummaryHeader when feature flag is enabled', async () => {
    localStorage.setItem(
      'featureFlag:fleet-console:android-health-metrics',
      'on',
    );

    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => ({
            totalDevices: 100,
            totalHosts: 10,
            labRunningHosts: 8,
            labMissingHosts: 2,
            healthCategoryInService: {
              total: 80,
              statusCounts: { IDLE: 50, BUSY: 30 },
            },
          }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(await screen.findByText('In Service')).toBeInTheDocument();
  });
});
