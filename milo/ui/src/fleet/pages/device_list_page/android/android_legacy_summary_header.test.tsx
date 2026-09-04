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

import { render, screen, fireEvent } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidLegacySummaryHeader } from './android_legacy_summary_header';
import { androidState } from './android_state';

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

describe('AndroidLegacySummaryHeader', () => {
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

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    // Verify that the title is rendered
    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();

    // Verify that some metrics are rendered
    expect(await screen.findByText('Total Devices')).toBeInTheDocument();
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

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Click on Idle metric
    const idleButton = await screen.findByRole('button', { name: 'Idle' });
    fireEvent.click(idleButton);

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

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Click on Init metric
    const initButton = await screen.findByRole('button', { name: 'Init' });
    fireEvent.click(initButton);

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

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const failedButton = await screen.findByRole('button', {
      name: 'Failed device_type',
    });
    fireEvent.click(failedButton);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: [
        androidState.IDLE,
        androidState.BUSY,
        androidState.LAMEDUCK,
      ],
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
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              idleDevices: 50,
              busyDevices: 30,
              lameduckDevices: 10,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const blankButton = await screen.findByRole('button', {
      name: 'Blank states',
    });
    fireEvent.click(blankButton);

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
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              totalHosts: 10,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const totalHostsButton = await screen.findByRole('button', {
      name: /Total Hosts/i,
    });
    fireEvent.click(totalHostsButton);

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
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const totalDevicesButton = await screen.findByRole('button', {
      name: /Total Devices/i,
    });
    fireEvent.click(totalDevicesButton);

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
              totalHosts: 0,
            },
          }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('Total Devices')).toBeInTheDocument();
    expect(screen.queryByText(/NaN/i)).not.toBeInTheDocument();
  });

  it('should call setFiltersBatch with Online filter when clicking Online', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
            },
          }),
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const onlineButton = await screen.findByRole('button', {
      name: /Online/i,
    });
    fireEvent.click(onlineButton);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should render 7 days avg and 30 days avg formatted as percentages when showAvgUtilization is true', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
              average7d: 0.8542,
              average30d: 0.9234,
            },
          }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('85.42%')).toBeInTheDocument();
    expect(screen.getByText('92.34%')).toBeInTheDocument();
  });

  it('should render "-" for 7 days avg and 30 days avg when utilization metrics are missing', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountDevices: {
        query: () => ({
          queryKey: ['CountDevices'],
          queryFn: async () => ({
            androidCount: {
              totalDevices: 100,
            },
          }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidLegacySummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    const dashes = await screen.findAllByText('-');
    expect(dashes.length).toBeGreaterThanOrEqual(2);
  });
});
