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

import {
  GetFleetAvailabilityTrendsResponse,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { PerformanceRankingCard } from './performance_ranking_card';
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

    expect(screen.getByRole('tab', { name: /^models$/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /^pools$/i })).toBeInTheDocument();

    // Verify ordering: volteer (62%) -> dedede (74%) -> brya (88%)
    const modelElements = screen.getAllByRole('listitem');
    expect(modelElements).toHaveLength(3);
    expect(modelElements[0]).toHaveTextContent('volteer');
    expect(modelElements[0]).toHaveTextContent('62% Available');
    expect(screen.getByTestId('drop-badge-volteer')).toHaveTextContent('-3%');

    expect(modelElements[1]).toHaveTextContent('dedede');
    expect(modelElements[1]).toHaveTextContent('74% Available');
    expect(screen.getByTestId('drop-badge-dedede')).toHaveTextContent('-1%');

    expect(modelElements[2]).toHaveTextContent('brya');
    expect(modelElements[2]).toHaveTextContent('88% Available');
    expect(screen.queryByTestId('drop-badge-brya')).not.toBeInTheDocument();
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

  it('calculates 24h delta by comparing against point 24 hours prior rather than previous hourly bucket', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            // 24h ago: value 0.80
            { timestamp: '2026-09-29T12:00:00Z', value: 0.8 },
            // 1h ago: value 0.55 (short-term dip)
            { timestamp: '2026-09-30T11:00:00Z', value: 0.55 },
            // Current reading: value 0.60 (up from 1h ago, but down 20% compared to 24h ago)
            { timestamp: '2026-09-30T12:00:00Z', value: 0.6 },
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

    // Compared to 24h ago (80%), current (60%) is a 20% drop, despite being up from 1h ago (55%)
    expect(screen.getByTestId('drop-badge-volteer')).toHaveTextContent('-20%');
  });

  it('calls onSelectModel when a model row is clicked', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.65 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.62 },
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

    const onSelectModel = jest.fn();

    render(
      <FakeContextProvider>
        <PerformanceRankingCard onSelectModel={onSelectModel} />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByTestId('ranking-row-volteer'));
    expect(onSelectModel).toHaveBeenCalledTimes(1);
    expect(onSelectModel).toHaveBeenCalledWith('volteer');
  });
  it('switches to Pools tab, fetches pool groupings, and renders sorted pools', () => {
    const mockPoolTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'DUT_POOL_QUOTA',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.92 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.85 },
          ],
        },
        {
          name: 'DUT_POOL_CQ',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.6 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.55 },
          ],
        },
      ],
    };

    const useTrendsSpy = jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: mockPoolTrends,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    const onSelectPool = jest.fn();

    render(
      <FakeContextProvider>
        <PerformanceRankingCard onSelectPool={onSelectPool} />
      </FakeContextProvider>,
    );

    const poolsTab = screen.getByRole('tab', { name: /^pools$/i });
    fireEvent.click(poolsTab);

    // Verify spy was called with TrendlineGrouping.GROUP_BY_POOL
    expect(useTrendsSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        grouping: TrendlineGrouping.GROUP_BY_POOL,
      }),
    );

    // Verify pool list ordering: DUT_POOL_CQ (55%) -> DUT_POOL_QUOTA (85%)
    const poolElements = screen.getAllByRole('listitem');
    expect(poolElements).toHaveLength(2);
    expect(poolElements[0]).toHaveTextContent('DUT_POOL_CQ');
    expect(poolElements[0]).toHaveTextContent('55% Healthy');
    expect(screen.getByTestId('drop-badge-DUT_POOL_CQ')).toHaveTextContent(
      '-5%',
    );

    expect(poolElements[1]).toHaveTextContent('DUT_POOL_QUOTA');
    expect(poolElements[1]).toHaveTextContent('85% Healthy');
    expect(screen.getByTestId('drop-badge-DUT_POOL_QUOTA')).toHaveTextContent(
      '-7%',
    );

    // Test clicking a pool row
    fireEvent.click(screen.getByTestId('ranking-row-DUT_POOL_CQ'));
    expect(onSelectPool).toHaveBeenCalledWith('DUT_POOL_CQ');
  });

  it('sorts cohorts by Health Drop descending when selected in Sort by dropdown', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.65 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.62 }, // drop: 3%
          ],
        },
        {
          name: 'dedede',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.85 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.75 }, // drop: 10%
          ],
        },
        {
          name: 'brya',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.8 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.8 }, // drop: 0%
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

    // Initial default sorting by Availability ascending: volteer (62%) -> dedede (75%) -> brya (80%)
    let rows = screen.getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('volteer');
    expect(rows[1]).toHaveTextContent('dedede');
    expect(rows[2]).toHaveTextContent('brya');

    // Switch sort to Health Drop
    const sortSelect = screen.getByRole('combobox', { name: /sort by/i });
    fireEvent.mouseDown(sortSelect);
    const dropOption = screen.getByRole('option', { name: /^health drop$/i });
    fireEvent.click(dropOption);

    // Sorted descending by health drop: dedede (-10%) -> volteer (-3%) -> brya (0%)
    rows = screen.getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('dedede');
    expect(rows[1]).toHaveTextContent('volteer');
    expect(rows[2]).toHaveTextContent('brya');
  });

  it('filters cohorts to only show degrading hardware when Drops Only is toggled', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.65 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.62 }, // drop: 3%
          ],
        },
        {
          name: 'brya',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.8 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.85 }, // no drop (improved)
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

    expect(screen.getAllByRole('listitem')).toHaveLength(2);

    const toggleButton = screen.getByRole('button', {
      name: /filter degraded models \(drops only\)/i,
    });
    fireEvent.click(toggleButton);

    // Only volteer should remain
    const filteredRows = screen.getAllByRole('listitem');
    expect(filteredRows).toHaveLength(1);
    expect(filteredRows[0]).toHaveTextContent('volteer');
    expect(screen.queryByText('brya')).not.toBeInTheDocument();

    // Toggle back
    fireEvent.click(
      screen.getByRole('button', {
        name: /showing degraded models — click to show all/i,
      }),
    );
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });
});
