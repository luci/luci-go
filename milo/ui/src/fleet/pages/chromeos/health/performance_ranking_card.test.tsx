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
import { render, screen } from '@testing-library/react';

import {
  GetFleetAvailabilityTrendsResponse,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { PerformanceRankingCard } from './performance_ranking_card';
import { getHealthColor } from './ranking_utils';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';

describe('PerformanceRankingCard', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders hardware models ordered ascending by availability % (worst first)', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'Overall',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.9 }],
        },
        {
          name: 'brya',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.85 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.88 },
          ],
        },
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.65 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.62 },
          ],
        },
        {
          name: 'dedede',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.75 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.74 },
          ],
        },
      ],
    };

    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: mockTrends,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <PerformanceRankingCard />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Hardware Performance')).toBeInTheDocument();
    expect(screen.getByText('Models')).toBeInTheDocument();

    // Verify ordering: volteer (62%) -> dedede (74%) -> brya (88%)
    const modelElements = screen.getAllByRole('listitem');
    expect(modelElements).toHaveLength(3);
    expect(modelElements[0]).toHaveTextContent('volteer');
    expect(modelElements[0]).toHaveTextContent('62% Available');
    expect(modelElements[1]).toHaveTextContent('dedede');
    expect(modelElements[1]).toHaveTextContent('74% Available');
    expect(modelElements[2]).toHaveTextContent('brya');
    expect(modelElements[2]).toHaveTextContent('88% Available');
  });

  it('renders loading state skeletons while fetching data', () => {
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

    const { container } = render(
      <FakeContextProvider>
        <PerformanceRankingCard />
      </FakeContextProvider>,
    );

    expect(
      container.querySelectorAll('.MuiSkeleton-root').length,
    ).toBeGreaterThan(0);
  });

  it('renders error alert on fetch failure', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: undefined,
        isLoading: false,
        isError: true,
        error: new Error('Network error'),
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <PerformanceRankingCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Failed to load hardware performance ranking.'),
    ).toBeInTheDocument();
  });

  it('renders empty state when no models are returned', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          metricType: TrendlineMetricType.AVAILABILITY,
          series: [],
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
        <PerformanceRankingCard />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('No models found matching current filters.'),
    ).toBeInTheDocument();
  });

  it('correctly maps SLA colors based on availability thresholds', () => {
    expect(getHealthColor(95)).toBe('success');
    expect(getHealthColor(85)).toBe('success');
    expect(getHealthColor(84)).toBe('warning');
    expect(getHealthColor(70)).toBe('warning');
    expect(getHealthColor(69)).toBe('error');
    expect(getHealthColor(0)).toBe('error');
  });
});
