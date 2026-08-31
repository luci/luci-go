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

import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidSummaryHeader } from './android_summary_header';

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
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();
    jest.clearAllMocks();
  });

  const setupMockClient = (androidCount: Record<string, unknown>) => {
    FleetConsoleMockAPI.setFixture('CountDevices', {
      androidCount,
    });
  };

  it('should render successfully with data', async () => {
    setupMockClient({
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
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
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
    setupMockClient({
      totalDevices: 100,
      idleDevices: 50,
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    // Click on Idle metric
    fireEvent.click(await screen.findByRole('button', { name: 'Idle' }));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['IDLE'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with Init state when clicking Init', async () => {
    setupMockClient({
      totalDevices: 100,
      initDevices: 5,
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    // Click on Init metric
    fireEvent.click(await screen.findByRole('button', { name: 'Init' }));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['INIT'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with IDLE, BUSY, LAMEDUCK when clicking Failed device_type', async () => {
    setupMockClient({});

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Failed device_type' }));

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['IDLE', 'BUSY', 'LAMEDUCK'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with (Blank) when clicking Blank states', async () => {
    setupMockClient({});

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Blank states' }));

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: ['(Blank)'],
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should clear state filter when clicking Total Hosts', async () => {
    setupMockClient({});

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    // Click on Total Hosts
    fireEvent.click(await screen.findByText('Total Hosts'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should clear state filter when clicking Total Devices', async () => {
    setupMockClient({});

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    // Click on Total Devices
    fireEvent.click(await screen.findByText('Total Devices'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should handle totalDevices: 0 without NaN% or crashing', async () => {
    setupMockClient({
      totalDevices: 0,
      idleDevices: 0,
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    // Verify that it renders without NaN%
    expect(screen.queryByText(/NaN/)).not.toBeInTheDocument();
  });

  it('should call setFiltersBatch with Online filter when clicking Online', async () => {
    setupMockClient({});

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader aip160="" setFiltersBatch={mockSetFiltersBatch} />
      </FakeContextProvider>,
    );

    // Click on Online
    fireEvent.click(await screen.findByText('Online'));

    // Verify that setFiltersBatch was called
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should render 7 days avg and 30 days avg formatted as percentages when showAvgUtilization is true', async () => {
    setupMockClient({
      totalDevices: 100,
      average7d: 0.17,
      average30d: 0.16,
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('17.00%')).toBeInTheDocument();
    expect(screen.getByText('16.00%')).toBeInTheDocument();
  });

  it('should render "-" for 7 days avg and 30 days avg when utilization metrics are missing', async () => {
    setupMockClient({
      totalDevices: 100,
    });

    render(
      <FakeContextProvider>
        <AndroidSummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    const dashElements = await screen.findAllByText('-');
    expect(dashElements.length).toBeGreaterThanOrEqual(2);
  });
});
