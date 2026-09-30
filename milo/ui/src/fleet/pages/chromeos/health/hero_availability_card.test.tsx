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

import { UseQueryResult } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import {
  GetFleetAvailabilityTrendsResponse,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { getHealthStatus } from './health_status_utils';
import { HeroAvailabilityCard } from './hero_availability_card';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';

describe('getHealthStatus', () => {
  it('returns Healthy for >= 95%', () => {
    expect(getHealthStatus(95)).toEqual({
      status: 'Healthy',
      color: 'success',
    });
    expect(getHealthStatus(100)).toEqual({
      status: 'Healthy',
      color: 'success',
    });
    expect(getHealthStatus(105)).toEqual({
      status: 'Healthy',
      color: 'success',
    });
  });

  it('returns Warning for 80% to 94%', () => {
    expect(getHealthStatus(80)).toEqual({
      status: 'Warning',
      color: 'warning',
    });
    expect(getHealthStatus(94)).toEqual({
      status: 'Warning',
      color: 'warning',
    });
  });

  it('returns Critical for < 80%', () => {
    expect(getHealthStatus(79)).toEqual({ status: 'Critical', color: 'error' });
    expect(getHealthStatus(0)).toEqual({ status: 'Critical', color: 'error' });
  });
});

describe('HeroAvailabilityCard', () => {
  const currentHourIso = DateTime.now().toUTC().startOf('hour').toISO()!;
  const pastHourIso = DateTime.now()
    .toUTC()
    .startOf('hour')
    .minus({ hours: 1 })
    .toISO()!;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders loading state when trends query is loading', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('hero-availability-card')).toBeInTheDocument();
    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('...');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'Loading...',
    );
  });

  it('renders no data state when series has no points', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [{ name: 'Overall', points: [] }],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('—');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'No data',
    );
  });

  it('extracts availability from the freshest data point and displays Healthy status', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [
                { timestamp: pastHourIso, value: 0.88 },
                { timestamp: currentHourIso, value: 0.96 },
              ],
            },
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Current Fleet Availability')).toBeInTheDocument();
    expect(screen.getByTestId('hero-baseline-chip')).toHaveTextContent(
      'Quota Baseline',
    );
    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('96%');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'Healthy (96%)',
    );
    expect(screen.getByText('Fleet Availability')).toBeInTheDocument();
  });

  it('correctly handles surplus availability exceeding 100% without rounding down', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 1.05 }],
            },
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('105%');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'Healthy (105%)',
    );
  });

  it('displays Critical status when availability is below 80%', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 0.74 }],
            },
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('74%');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'Critical (74%)',
    );
  });

  it('switches to Enrolled Baseline when server returns HEALTH metricType', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 0.91 }],
            },
          ],
          metricType: TrendlineMetricType.HEALTH,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Current Filtered Health')).toBeInTheDocument();
    expect(screen.getByTestId('hero-baseline-chip')).toHaveTextContent(
      'Enrolled Baseline',
    );
    expect(screen.getByText('Fleet Health')).toBeInTheDocument();
    expect(
      screen.getByTestId('hero-availability-percentage'),
    ).toHaveTextContent('91%');
    expect(screen.getByTestId('hero-status-label')).toHaveTextContent(
      'Warning (91%)',
    );
  });

  it('renders help icon tooltip with dut_state = READY explanation', async () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 0.95 }],
            },
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    const helpIcon = screen.getByTestId('hero-help-icon');
    expect(helpIcon).toBeInTheDocument();
    fireEvent.mouseOver(helpIcon);
    expect(
      await screen.findByText(/Availability vs. Health:/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/dut_state = READY/i)).toBeInTheDocument();
  });

  it('renders baseline chip tooltip on hover', async () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          series: [
            {
              name: 'Overall',
              points: [{ timestamp: currentHourIso, value: 0.95 }],
            },
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HeroAvailabilityCard />
      </FakeContextProvider>,
    );

    const chip = screen.getByTestId('hero-baseline-chip');
    expect(chip).toBeInTheDocument();
    fireEvent.mouseOver(chip);
    expect(
      await screen.findByText(
        /Availability is calculated as the percentage of READY devices/i,
      ),
    ).toBeInTheDocument();
  });
});
