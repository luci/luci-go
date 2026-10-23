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

import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import {
  GetDeviceDimensionsResponse,
  GetFleetAvailabilityTrendsResponse,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { PerformanceRankingCard } from './performance_ranking_card';
import { HEALTH_COMPARISON_TOOLTIP } from './ranking_utils';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';

describe('PerformanceRankingCard', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders hardware models ordered by health drop descending by default (worst first)', () => {
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
    expect(
      screen.getByRole('combobox', { name: /sort by/i }),
    ).toHaveTextContent('Health Drop');

    // Verify ordering by health drop descending: volteer (-3%) -> dedede (-1%) -> brya (+3% gain, 0% drop)
    const modelElements = screen.getAllByRole('listitem');
    expect(modelElements).toHaveLength(3);
    expect(modelElements[0]).toHaveTextContent('volteer');
    expect(modelElements[0]).toHaveTextContent('62.0% Available');
    expect(screen.getByTestId('drop-badge-volteer')).toHaveTextContent('-3.0%');

    expect(modelElements[1]).toHaveTextContent('dedede');
    expect(modelElements[1]).toHaveTextContent('74.0% Available');
    expect(screen.getByTestId('drop-badge-dedede')).toHaveTextContent('-1.0%');

    expect(modelElements[2]).toHaveTextContent('brya');
    expect(modelElements[2]).toHaveTextContent('88.0% Available');
    expect(screen.queryByTestId('drop-badge-brya')).not.toBeInTheDocument();
    expect(screen.getByTestId('gain-badge-brya')).toHaveTextContent('+3.0%');
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
    expect(screen.getByTestId('drop-badge-volteer')).toHaveTextContent(
      '-20.0%',
    );
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

    // Verify spy was called with TrendlineGrouping.GROUP_BY_POOL and 26-hour aligned window
    expect(useTrendsSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        grouping: TrendlineGrouping.GROUP_BY_POOL,
        startTime: expect.stringMatching(
          /^\d{4}-\d{2}-\d{2}T\d{2}:00:00\.000Z$/,
        ),
        endTime: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T\d{2}:00:00\.000Z$/),
      }),
    );

    // Verify pool list ordering by health drop descending: DUT_POOL_QUOTA (-7%) -> DUT_POOL_CQ (-5%)
    const poolElements = screen.getAllByRole('listitem');
    expect(poolElements).toHaveLength(2);
    expect(poolElements[0]).toHaveTextContent('DUT_POOL_QUOTA');
    expect(poolElements[0]).toHaveTextContent('85.0% Healthy');
    expect(screen.getByTestId('drop-badge-DUT_POOL_QUOTA')).toHaveTextContent(
      '-7.0%',
    );

    expect(poolElements[1]).toHaveTextContent('DUT_POOL_CQ');
    expect(poolElements[1]).toHaveTextContent('55.0% Healthy');
    expect(screen.getByTestId('drop-badge-DUT_POOL_CQ')).toHaveTextContent(
      '-5.0%',
    );

    // Test clicking a pool row
    fireEvent.click(screen.getByTestId('ranking-row-DUT_POOL_CQ'));
    expect(onSelectPool).toHaveBeenCalledWith('DUT_POOL_CQ');
  });

  it('sorts cohorts by Health Drop descending by default and sorts by Availability ascending when selected in Sort by dropdown', () => {
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

    // Initial default sorting by Health Drop descending: dedede (-10%) -> volteer (-3%) -> brya (0%)
    let rows = screen.getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('dedede');
    expect(rows[1]).toHaveTextContent('volteer');
    expect(rows[2]).toHaveTextContent('brya');

    // Switch sort to Availability
    const sortSelect = screen.getByRole('combobox', { name: /sort by/i });
    fireEvent.mouseDown(sortSelect);
    const availabilityOption = screen.getByRole('option', {
      name: /^availability$/i,
    });
    fireEvent.click(availabilityOption);

    // Sorted ascending by availability: volteer (62%) -> dedede (75%) -> brya (80%)
    rows = screen.getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('volteer');
    expect(rows[1]).toHaveTextContent('dedede');
    expect(rows[2]).toHaveTextContent('brya');
  });

  it('renders View devices button linking to filtered devices page and prevents row click', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.62 }],
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

    const devicesLink = screen.getByRole('link', { name: /^view devices$/i });
    const href = devicesLink.getAttribute('href') ?? '';
    const url = new URL(href, 'http://localhost');
    expect(url.pathname).toBe('/ui/fleet/p/chromeos/devices');
    expect(url.searchParams.get('filters')).toBe(
      '(labels."label-model" = "volteer")',
    );

    // Clicking the link should NOT fire row click onSelectModel
    fireEvent.click(devicesLink);
    expect(onSelectModel).not.toHaveBeenCalled();
  });

  it('uses device dimension casing for model names in link and row click', () => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {},
          labels: {
            'label-model': { values: ['Anahera', 'Brya'] },
          },
        } as unknown as GetDeviceDimensionsResponse,
        isPending: false,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: {
          metricType: TrendlineMetricType.AVAILABILITY,
          series: [
            {
              name: 'anahera',
              points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.62 }],
            },
          ],
        },
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

    const devicesLink = screen.getByRole('link', { name: /^view devices$/i });
    const url = new URL(
      devicesLink.getAttribute('href') ?? '',
      'http://localhost',
    );
    expect(url.searchParams.get('filters')).toBe(
      '(labels."label-model" = "Anahera")',
    );

    fireEvent.click(screen.getByTestId('ranking-row-anahera'));
    expect(onSelectModel).toHaveBeenCalledWith('Anahera');
  });

  it('displays comparison tooltip when hovering over health drop and gain badges', async () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.8 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.6 },
          ],
        },
        {
          name: 'brya',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.7 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.9 },
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

    // Hover over drop badge
    const dropBadge = screen.getByTestId('drop-badge-volteer');
    fireEvent.mouseOver(dropBadge);
    expect(
      await screen.findByText(HEALTH_COMPARISON_TOOLTIP),
    ).toBeInTheDocument();

    // Hover over gain badge
    const gainBadge = screen.getByTestId('gain-badge-brya');
    fireEvent.mouseOver(gainBadge);
    expect(
      await screen.findByText(HEALTH_COMPARISON_TOOLTIP),
    ).toBeInTheDocument();
  });

  it('displays state breakdown popover with SLA thresholds when hovering over progress bar', async () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            {
              timestamp: '2026-09-30T00:00:00Z',
              value: 0.7,
              stateCounts: {
                ready: 70,
                needRepair: 10,
                repairFailed: 0,
                needManualRepair: 20,
                needsReplacement: 0,
                reserved: 0,
                needsDeploy: 0,
              },
            },
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

    const progressBar = screen.getByTestId('progress-bar-volteer');
    fireEvent.mouseEnter(progressBar);

    expect(
      await screen.findByTestId('popover-spectrum-bar'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('state-row-ready')).toBeInTheDocument();
    expect(screen.getByTestId('state-row-needs_repair')).toBeInTheDocument();
    expect(
      screen.getByTestId('state-row-needs_manual_repair'),
    ).toBeInTheDocument();
  });

  it('displays state breakdown popover when hovering over availability label', async () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [{ timestamp: '2026-09-30T00:00:00Z', value: 0.7 }],
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

    const availabilityLabel = screen.getByTestId('availability-label-volteer');
    fireEvent.mouseEnter(availabilityLabel);
    expect(
      await screen.findByTestId('popover-empty-state'),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('popover-thresholds-strip'),
    ).not.toBeInTheDocument();
    expect(screen.getByText('No devices enrolled')).toBeInTheDocument();
  });

  it('displays comparison tooltip when hovering over flat trend badge', async () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.8 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.8 },
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

    const flatBadge = screen.getByTestId('flat-badge-volteer');
    fireEvent.mouseOver(flatBadge);
    expect(
      await screen.findByText(HEALTH_COMPARISON_TOOLTIP),
    ).toBeInTheDocument();
  });

  it('triggers onSelectModel when clicking on drop badge', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T00:00:00Z', value: 0.9 },
            { timestamp: '2026-09-30T00:00:00Z', value: 0.75 },
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

    const dropBadge = screen.getByTestId('drop-badge-volteer');
    fireEvent.click(dropBadge);
    expect(onSelectModel).toHaveBeenCalledWith('volteer');
  });

  it('correctly handles surplus availability exceeding 100% without capping at 100%', () => {
    const mockTrends: GetFleetAvailabilityTrendsResponse = {
      metricType: TrendlineMetricType.AVAILABILITY,
      series: [
        {
          name: 'volteer',
          points: [
            { timestamp: '2026-09-29T12:00:00Z', value: 1.25 },
            { timestamp: '2026-09-30T12:00:00Z', value: 1.15 },
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

    const modelElements = screen.getAllByRole('listitem');
    expect(modelElements).toHaveLength(1);
    expect(modelElements[0]).toHaveTextContent('volteer');
    expect(modelElements[0]).toHaveTextContent('115.0% Available');
    expect(screen.getByTestId('drop-badge-volteer')).toHaveTextContent(
      '-10.0%',
    );

    const progressBar = screen.getByTestId('progress-bar-volteer');
    expect(progressBar).toHaveAttribute('role', 'progressbar');
  });
});
