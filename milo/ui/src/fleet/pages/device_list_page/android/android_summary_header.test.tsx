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
  FC_IS_OFFLINE: '"fc_is_offline"',
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
    expect(screen.getByText('Hosts Running')).toBeInTheDocument();
    expect(screen.getByText('Hosts Missing')).toBeInTheDocument();
    expect(screen.getByText('Online')).toBeInTheDocument();
    expect(screen.getByText('Offline')).toBeInTheDocument();
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

    // Click on Idle metric
    fireEvent.click(screen.getByRole('button', { name: 'Idle' }));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['IDLE'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with Init state when clicking Init', async () => {
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

    // Click on Init metric
    fireEvent.click(screen.getByRole('button', { name: 'Init' }));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['INIT'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with IDLE, BUSY, LAMEDUCK when clicking Failed device_type', async () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Failed device_type' }));

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['IDLE', 'BUSY', 'LAMEDUCK'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with (Blank) when clicking Blank states', async () => {
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

    fireEvent.click(screen.getByRole('button', { name: 'Blank states' }));

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['(Blank)'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
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

  it('should call setFiltersBatch with Online filter when clicking Online', async () => {
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

    // Click on Online
    fireEvent.click(screen.getByText('Online'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });
});
