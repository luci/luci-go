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

import { render, screen, fireEvent, within } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  AndroidHealthSummaryHeader,
  SHOW_ALL_STATES_STORAGE_KEY,
} from './android_health_summary_header';
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
  HEALTH_CATEGORY: '"health_category"',
} as const;

describe('AndroidHealthSummaryHeader', () => {
  const mockHealthResponse = {
    totalDevices: 28748,
    totalHosts: 3181,
    labRunningHosts: 3101,
    labMissingHosts: 80,
    healthCategoryInService: {
      total: 24199,
      statusCounts: {
        IDLE: 16859,
        BUSY: 7215,
        LAMEDUCK: 125,
        BLANK: 0,
      },
      average7d: 0.912,
      average30d: 0.895,
    },
    healthCategoryInTransition: {
      total: 1000,
      statusCounts: {
        INIT: 500,
        PREPPING: 500,
      },
    },
    healthCategoryInAutoRecovery: {
      total: 1549,
      statusCounts: {
        DIRTY: 1549,
      },
    },
    healthCategoryNeedManualRepair: {
      total: 2000,
      statusCounts: {
        MISSING: 1500,
        FAILED: 400,
        DYING: 100,
      },
    },
    healthCategoryUnspecified: {
      total: 100,
      statusCounts: {
        INIT: 100,
      },
    },
    average7d: 0.842,
    average30d: 0.815,
  };

  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
  });

  it('should render health metric buckets successfully', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(screen.getByText('Total Hosts')).toBeInTheDocument();
    expect(screen.getByText('Hosts Running')).toBeInTheDocument();
    expect(screen.getByText('Hosts Missing')).toBeInTheDocument();

    expect(await screen.findByText('Total Devices')).toBeInTheDocument();
    expect(screen.getByText('In Service')).toBeInTheDocument();
    expect(screen.getByText('Need Manual Repair')).toBeInTheDocument();
    expect(screen.getByText('In Automated Maintenance')).toBeInTheDocument();

    // Total Devices utilization is shown
    expect(await screen.findByText('84.20%')).toBeInTheDocument();
    expect(screen.getByText('81.50%')).toBeInTheDocument();

    // Default collapsed view: no statuses should be shown
    expect(screen.queryByText('Idle:')).not.toBeInTheDocument();
    expect(screen.queryByText('Busy:')).not.toBeInTheDocument();
    expect(screen.queryByText('Lameduck:')).not.toBeInTheDocument();
    expect(screen.queryByText('Dirty:')).not.toBeInTheDocument();
    expect(screen.queryByText('Prepping:')).not.toBeInTheDocument();

    // Expand view
    fireEvent.click(screen.getByRole('checkbox', { name: 'Show all states' }));
    expect(screen.getAllByText('Idle:').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Busy:').length).toBeGreaterThan(0);
    expect(screen.getByText('Lameduck:')).toBeInTheDocument();
    expect(screen.getByText('Dirty:')).toBeInTheDocument();
    expect(screen.getByText('Prepping:')).toBeInTheDocument();
  });

  it('should call setFiltersBatch when clicking In Service main metric', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const inServiceBtn = await screen.findByRole('button', {
      name: /In Service/i,
    });
    fireEvent.click(inServiceBtn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: ['HEALTH_CATEGORY_IN_SERVICE'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should call setFiltersBatch when clicking In Automated Maintenance main metric', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const btn = await screen.findByRole('button', {
      name: /In Automated Maintenance/i,
    });
    fireEvent.click(btn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: [
        'HEALTH_CATEGORY_IN_TRANSITION',
        'HEALTH_CATEGORY_IN_AUTO_RECOVERY',
        'HEALTH_CATEGORY_UNSPECIFIED',
      ],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should call setFiltersBatch when clicking a status breakdown item inside In Automated Maintenance', async () => {
    localStorage.setItem(SHOW_ALL_STATES_STORAGE_KEY, 'true');
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const maintSection = await screen.findByLabelText(
      'In Automated Maintenance Devices',
    );
    await within(maintSection).findByText('1,549');
    const dirtyBtn = within(maintSection).getByRole('button', {
      name: 'Dirty',
    });
    fireEvent.click(dirtyBtn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: [
        'HEALTH_CATEGORY_IN_TRANSITION',
        'HEALTH_CATEGORY_IN_AUTO_RECOVERY',
        'HEALTH_CATEGORY_UNSPECIFIED',
      ],
      [FILTER_KEYS.STATE]: ['DIRTY'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch when clicking Need Manual Repair main metric', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const btn = await screen.findByRole('button', {
      name: /Need Manual Repair/i,
    });
    fireEvent.click(btn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: ['HEALTH_CATEGORY_NEED_MANUAL_REPAIR'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
      [FILTER_KEYS.STATE]: [],
    });
  });

  it('should call setFiltersBatch when clicking a status breakdown item inside In Service', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Click "Show all states" switch to reveal status breakdown
    const toggleSwitch = await screen.findByRole('checkbox', {
      name: 'Show all states',
    });
    fireEvent.click(toggleSwitch);

    const inServiceSection = await screen.findByLabelText('In Service Devices');
    await within(inServiceSection).findByText('16,859');
    const idleBtn = within(inServiceSection).getByRole('button', {
      name: 'Idle',
    });
    fireEvent.click(idleBtn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: ['HEALTH_CATEGORY_IN_SERVICE'],
      [FILTER_KEYS.STATE]: ['IDLE'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should call setFiltersBatch with (Blank) when clicking Blank states after showing all states', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Click "Show all states" switch
    const toggleSwitch = await screen.findByRole('checkbox', {
      name: 'Show all states',
    });
    fireEvent.click(toggleSwitch);

    const inServiceSection = await screen.findByLabelText('In Service Devices');
    const blankBtn = within(inServiceSection).getByRole('button', {
      name: 'Blank states',
    });
    fireEvent.click(blankBtn);

    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.HEALTH_CATEGORY]: ['HEALTH_CATEGORY_IN_SERVICE'],
      [FILTER_KEYS.STATE]: ['(Blank)'],
      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
    });
  });

  it('should toggle show all states and persist state to localStorage', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    // Initial state: "Show all states" switch is unchecked, states are not visible
    const toggleSwitch = await screen.findByRole('checkbox', {
      name: 'Show all states',
    });
    expect(toggleSwitch).toBeInTheDocument();
    expect(toggleSwitch).not.toBeChecked();
    expect(screen.queryByText('Idle:')).not.toBeInTheDocument();
    expect(screen.queryByText('Lameduck:')).not.toBeInTheDocument();

    // Click switch to show all states
    fireEvent.click(toggleSwitch);

    // All states should now be visible
    expect(toggleSwitch).toBeChecked();
    expect(screen.getAllByText('Idle:').length).toBeGreaterThan(0);
    expect(screen.getByText('Lameduck:')).toBeInTheDocument();
    expect(screen.getByText('Dirty:')).toBeInTheDocument();
    expect(screen.getByText('Prepping:')).toBeInTheDocument();
    expect(localStorage.getItem(SHOW_ALL_STATES_STORAGE_KEY)).toBe('true');

    // Click again to hide other states
    fireEvent.click(toggleSwitch);
    expect(toggleSwitch).not.toBeChecked();
    expect(screen.queryByText('Idle:')).not.toBeInTheDocument();
    expect(screen.queryByText('Lameduck:')).not.toBeInTheDocument();
    expect(localStorage.getItem(SHOW_ALL_STATES_STORAGE_KEY)).toBe('false');
  });

  it('should initialize showAllStates from localStorage', async () => {
    localStorage.setItem(SHOW_ALL_STATES_STORAGE_KEY, 'true');

    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    // Since localStorage was 'true', other states should be visible right away
    const toggleSwitch = await screen.findByRole('checkbox', {
      name: 'Show all states',
    });
    expect(toggleSwitch).toBeChecked();
    expect(screen.getAllByText('Lameduck:').length).toBeGreaterThan(0);
  });

  it('should clear state and health category filters when clicking Total Devices', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
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
      [FILTER_KEYS.HEALTH_CATEGORY]: [],
    });
  });

  it('should clear state and health category filters when clicking Total Hosts', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
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
      [FILTER_KEYS.HEALTH_CATEGORY]: [],
    });
  });

  it('should filter by Hosts Running and Hosts Missing correctly', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: async () => mockHealthResponse,
        }),
      },
    });

    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    const runningBtn = await screen.findByRole('button', {
      name: /Hosts Running/i,
    });
    fireEvent.click(runningBtn);
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: [androidState.LAB_RUNNING],
      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
      [FILTER_KEYS.HEALTH_CATEGORY]: [],
    });

    const missingBtn = await screen.findByRole('button', {
      name: /Hosts Missing/i,
    });
    fireEvent.click(missingBtn);
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      [FILTER_KEYS.STATE]: [androidState.LAB_MISSING],
      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
      [FILTER_KEYS.HEALTH_CATEGORY]: [],
    });
  });

  it('should render Total Devices utilization metrics when showAvgUtilization is true', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: () => Promise.resolve(mockHealthResponse),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={true}
        />
      </FakeContextProvider>,
    );

    expect(await screen.findByText('84.20%')).toBeInTheDocument();
    expect(screen.getByText('81.50%')).toBeInTheDocument();
  });

  it('should not render category utilization metrics when showAvgUtilization is false', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: () => Promise.resolve(mockHealthResponse),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader
          aip160=""
          setFiltersBatch={jest.fn()}
          showAvgUtilization={false}
        />
      </FakeContextProvider>,
    );

    expect(
      await screen.findByText('Device Health Metrics'),
    ).toBeInTheDocument();
    expect(screen.queryByText('91.20%')).not.toBeInTheDocument();
    expect(screen.queryByText('89.50%')).not.toBeInTheDocument();
    expect(screen.queryByText('84.20%')).not.toBeInTheDocument();
  });

  it('should render state placeholders when loading with showAllStates enabled', async () => {
    localStorage.setItem(SHOW_ALL_STATES_STORAGE_KEY, 'true');
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      CountAndroidDevices: {
        query: () => ({
          queryKey: ['CountAndroidDevices'],
          queryFn: () => new Promise(() => {}),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <AndroidHealthSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();
    expect(screen.getAllByText('Idle:').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Busy:').length).toBeGreaterThan(0);
  });
});
