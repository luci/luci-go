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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, fireEvent } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

import { AndroidSummaryHeader } from './android_summary_header';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

const mockNavigate = jest.fn();
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useNavigate: () => mockNavigate,
}));

const FILTER_KEYS = {
  STATE: '"state"',
  MACHINE_TYPE: '"fc_machine_type"',
} as const;

describe('AndroidSummaryHeader', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should render successfully with data', async () => {
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
              missingDevices: 5,
              failedDevices: 5,
              dirtyDevices: 5,
              preppingDevices: 3,
              dyingDevices: 2,
              initDevices: 5,
              lameduckDevices: 5,
              labRunningHosts: 8,
              labMissingHosts: 2,
            },
          }),
        }),
      },
    });

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </QueryClientProvider>,
    );

    // Verify that the title is rendered
    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();

    // Verify that some metrics are rendered
    expect(screen.getByText('Total Devices')).toBeInTheDocument();
    expect(screen.getByText('Total Hosts')).toBeInTheDocument();
    expect(screen.getByText('Total Healthy')).toBeInTheDocument();
    expect(screen.getByText('Total Unhealthy')).toBeInTheDocument();

    // Verify that Recovering state is rendered
    expect(screen.getByText(/Recovering/)).toBeInTheDocument();
  });

  it('should call setFiltersBatch when clicking a breakdown item and switch scope', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              idleDevices: 50,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </QueryClientProvider>,
    );

    // Click on Idle metric using role
    fireEvent.click(screen.getByRole('button', { name: /Idle/ }));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['IDLE'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with all recovering states when clicking Recovering', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              initDevices: 5,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </QueryClientProvider>,
    );

    // Click on Recovering metric using role
    fireEvent.click(screen.getByRole('button', { name: /Recovering/ }));

    // Verify that setFiltersBatch was called with all 4 states
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['INIT', 'DIRTY', 'PREPPING', 'LAMEDUCK'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should clear state filter when clicking Total Hosts', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({ androidCount: {} }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();
    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </QueryClientProvider>,
    );

    // Click on Total Hosts
    fireEvent.click(screen.getByText('Total Hosts'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should clear state filter when clicking Total Devices', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({ androidCount: {} }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();
    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </QueryClientProvider>,
    );

    // Click on Total Devices
    fireEvent.click(screen.getByText('Total Devices'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should handle totalDevices: 0 without NaN% or crashing', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 0,
              idleDevices: 0,
            },
          }),
        }),
      },
    });

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </QueryClientProvider>,
    );

    // Verify that it renders without NaN%
    expect(screen.queryByText(/NaN/)).not.toBeInTheDocument();
  });

  it('should call setFiltersBatch with all healthy states when clicking Total Healthy', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({ androidCount: {} }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();
    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </QueryClientProvider>,
    );

    // Click on Total Healthy
    fireEvent.click(screen.getByText('Total Healthy'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: [
        'IDLE',
        'BUSY',
        'INIT',
        'DIRTY',
        'PREPPING',
        'LAMEDUCK',
      ],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });
});
