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

import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSSummaryHeader } from './chromeos_summary_header';

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

const mockNavigate = jest.fn();
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useNavigate: () => mockNavigate,
}));

describe('ChromeOSSummaryHeader', () => {
  beforeEach(() => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();
    jest.clearAllMocks();
  });

  const setupMockClient = (deviceStateCounts: Record<string, number>) => {
    FleetConsoleMockAPI.setFixture('CountDevices', {
      total: 100,
      taskState: {
        idle: 60,
        busy: 40,
      },
      deviceState: {
        ready: 80,
        needRepair: 5,
        repairFailed: 2,
        needManualRepair: 1,
        needsDeploy: 1,
        needsReplacement: 1,
        reserved: 5,
        ...deviceStateCounts,
      },
    });
  };

  it('should render successfully with data', async () => {
    setupMockClient({});

    render(
      <FakeContextProvider>
        <ChromeOSSummaryHeader aip160="" setFiltersBatch={jest.fn()} />
      </FakeContextProvider>,
    );

    // Verify that the title is rendered
    expect(screen.getByText('Device Health Metrics')).toBeInTheDocument();

    // Scoped to Devices container
    const devicesContainer = screen.getByTestId('devices-metrics-grid');

    const reservedButton = within(devicesContainer).getByRole('button', {
      name: 'Reserved',
    });
    // Wait for dynamic values to load
    await within(reservedButton).findByText('5');
    expect(reservedButton).toHaveTextContent('Reserved:5(5.0%)');

    const otherStatesButton = within(devicesContainer).getByRole('button', {
      name: 'Other states',
    });
    await within(otherStatesButton).findByText('5');
    expect(otherStatesButton).toHaveTextContent('Other states:5(5.0%)');
  });

  it('should call setFiltersBatch with reserved when clicking Reserved', async () => {
    setupMockClient({ reserved: 8 });
    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <ChromeOSSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Click on Reserved metric (only exists in Devices card, so it is unique)
    fireEvent.click(await screen.findByRole('button', { name: 'Reserved' }));

    // Verify that setFiltersBatch was called with reserved
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      'labels."dut_state"': ['reserved'],
    });
  });

  it('should call setFiltersBatch with unknown and registered when clicking Other states', async () => {
    setupMockClient({});
    const mockSetFiltersBatch = jest.fn();

    render(
      <FakeContextProvider>
        <ChromeOSSummaryHeader
          aip160=""
          setFiltersBatch={mockSetFiltersBatch}
        />
      </FakeContextProvider>,
    );

    // Scoped to Devices container to avoid matching Labstations' "Other states" button
    const devicesContainer = screen.getByTestId('devices-metrics-grid');
    const otherStatesButton = within(devicesContainer).getByRole('button', {
      name: 'Other states',
    });

    // Click on Other states metric
    fireEvent.click(otherStatesButton);

    // Verify that setFiltersBatch was called with unknown and registered (excluding reserved)
    expect(mockSetFiltersBatch).toHaveBeenCalledWith({
      'labels."dut_state"': ['unknown', 'registered'],
    });
  });
});
