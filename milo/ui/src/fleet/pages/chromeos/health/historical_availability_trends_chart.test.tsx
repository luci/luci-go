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
import { fireEvent, render, screen, within } from '@testing-library/react';

import {
  GetFleetAvailabilityTrendsResponse,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HistoricalAvailabilityTrendsChart } from './historical_availability_trends_chart';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';

// `ResponsiveContainer` sizes itself from the DOM, and jsdom reports every
// element as 0x0, which would leave recharts nothing to draw. Handing the
// chart fixed dimensions is all the stub does; every other recharts component
// is the real one, so these tests still assert against real SVG.
jest.mock('recharts', () => {
  const actual = jest.requireActual('recharts');
  const react = jest.requireActual<typeof import('react')>('react');
  return {
    ...actual,
    ResponsiveContainer: ({
      children,
    }: {
      children: React.ReactElement<{ width?: number; height?: number }>;
    }) => react.cloneElement(children, { width: 800, height: 300 }),
  };
});

describe('HistoricalAvailabilityTrendsChart', () => {
  const t0 = '2026-09-14T10:00:00Z';
  const t1 = '2026-09-14T11:00:00Z';

  const mockOverallData: GetFleetAvailabilityTrendsResponse = {
    series: [
      {
        name: 'Overall',
        points: [
          { timestamp: t0, value: 0.925 },
          { timestamp: t1, value: 0.95 },
        ],
      },
    ],
    metricType: TrendlineMetricType.HEALTH,
  };

  const mockModelData: GetFleetAvailabilityTrendsResponse = {
    series: [
      {
        name: 'brya',
        points: [
          { timestamp: t0, value: 0.9 },
          { timestamp: t1, value: 1.0 },
        ],
      },
      {
        name: 'volteer',
        points: [
          { timestamp: t0, value: 0.833 },
          { timestamp: t1, value: 0.917 },
        ],
      },
    ],
    metricType: TrendlineMetricType.AVAILABILITY,
  };

  const mockPoolData: GetFleetAvailabilityTrendsResponse = {
    series: [
      {
        name: 'DUT_POOL_QUOTA',
        points: [{ timestamp: t0, value: 0.96 }],
      },
    ],
    metricType: TrendlineMetricType.HEALTH,
  };

  /**
   * Gives every element a non-zero box. Recharts locates the pointer from
   * `getBoundingClientRect`, and divides that width by `offsetWidth` to work
   * out whether CSS has scaled the chart. jsdom reports zero for both, which
   * would make that ratio infinite and every coordinate NaN.
   *
   * Side effect worth knowing: recharts also measures tick label widths with
   * `getBoundingClientRect`, so under this stub every label claims to be 800px
   * wide and the axis culls all but one of them. Harmless here, but do not
   * assert on axis labels in a test that calls this.
   */
  const giveChartASize = () => {
    jest.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 800,
      height: 300,
      top: 0,
      left: 0,
      right: 800,
      bottom: 300,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);
    jest
      .spyOn(HTMLElement.prototype, 'offsetWidth', 'get')
      .mockReturnValue(800);
    jest
      .spyOn(HTMLElement.prototype, 'offsetHeight', 'get')
      .mockReturnValue(300);
  };

  afterEach(() => {
    jest.restoreAllMocks();
  });

  /**
   * Everything recharts draws for a series: its line, and a dot per reading.
   * All of them carry the series' test id, and all of them are stroked in its
   * color.
   */
  const partsOf = (name: string) =>
    screen.queryAllByTestId(`series-line-${name}`);

  const isPlotted = (name: string) => partsOf(name).length > 0;

  describe('plotted geometry', () => {
    const asQueryResult = (data: GetFleetAvailabilityTrendsResponse) =>
      ({
        data,
        isLoading: false,
        isError: false,
        error: null,
      }) as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >;

    const hour = (h: number) =>
      `2026-09-14T${String(h).padStart(2, '0')}:00:00Z`;

    /** The `d` attribute of a series' line, or undefined if it has none. */
    const pathOf = (container: HTMLElement, name: string) =>
      container
        .querySelector(`[data-testid="series-line-${name}"].recharts-curve`)
        ?.getAttribute('d') ?? undefined;

    /** The x coordinate of every command in a path. */
    const xsOf = (d: string) =>
      [...d.matchAll(/[ML]\s*([\d.]+)/g)].map((m) => Number(m[1]));

    const renderOverall = (
      points: readonly { timestamp: string; value: number }[],
    ) => {
      jest
        .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
        .mockReturnValue(
          asQueryResult({
            series: [{ name: 'Overall', points }],
            metricType: TrendlineMetricType.HEALTH,
          }),
        );
      return render(
        <FakeContextProvider>
          <HistoricalAvailabilityTrendsChart />
        </FakeContextProvider>,
      );
    };

    it('breaks the line across a bucket with no reading', () => {
      // A cohort that reported nothing at 11:00 must not be drawn through
      // that hour, so the line is two subpaths rather than one.
      const { container } = renderOverall([
        { timestamp: hour(10), value: 0.9 },
        { timestamp: hour(12), value: 0.8 },
        { timestamp: hour(13), value: 0.85 },
      ]);

      const d = pathOf(container, 'Overall');
      expect(d).toBeDefined();
      expect(d!.match(/M/g)).toHaveLength(2);
    });

    it('spaces buckets by elapsed time, not by array position', () => {
      // Two steps of one hour, then a six hour outage. On the old ordinal
      // axis all three gaps were the same width.
      const { container } = renderOverall([
        { timestamp: hour(10), value: 0.9 },
        { timestamp: hour(11), value: 0.9 },
        { timestamp: hour(17), value: 0.9 },
      ]);

      const xs = xsOf(pathOf(container, 'Overall')!);
      expect(xs).toHaveLength(3);

      const oneHourStep = xs[1] - xs[0];
      const sixHourStep = xs[2] - xs[1];
      expect(oneHourStep).toBeGreaterThan(0);
      expect(sixHourStep / oneHourStep).toBeCloseTo(6, 1);
    });

    it('still marks a reading that stands alone between empty buckets', () => {
      // An isolated reading has no neighbour to draw a segment to, so the
      // line alone would render nothing at all for it.
      const { container } = renderOverall([
        { timestamp: hour(10), value: 0.9 },
        { timestamp: hour(14), value: 0.5 },
        { timestamp: hour(18), value: 0.7 },
      ]);

      const dots = container.querySelectorAll(
        '[data-testid="series-line-Overall"].recharts-dot',
      );
      expect(dots).toHaveLength(3);
    });

    it('draws no marker in buckets where the series reported nothing', () => {
      const { container } = renderOverall([
        { timestamp: hour(10), value: 0.9 },
        { timestamp: hour(13), value: 0.9 },
      ]);

      // Four buckets span 10:00 to 13:00, but only two carry a reading; the
      // empty ones must not be drawn at zero.
      const dots = container.querySelectorAll(
        '[data-testid="series-line-Overall"].recharts-dot',
      );
      expect(dots).toHaveLength(2);
    });
  });

  it('renders overall view by default with header, chip, and chart', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((req) => {
        expect(req.grouping).toBe(TrendlineGrouping.GROUP_BY_OVERALL);
        return {
          data: mockOverallData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    expect(
      screen.getByRole('heading', {
        name: 'Fleet Availability & Health Trends',
      }),
    ).toBeInTheDocument();
    expect(screen.getByText('Last 72h')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Overall fleet health percentage trendline over the last 72 hours.',
      ),
    ).toBeInTheDocument();

    expect(screen.getByTestId('trends-svg-chart')).toBeInTheDocument();
    expect(isPlotted('Overall')).toBe(true);
  });

  it('switches to By Model tab and displays model series selector checkboxes', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((req) => {
        if (req.grouping === TrendlineGrouping.GROUP_BY_MODEL) {
          return {
            data: mockModelData,
            isLoading: false,
            isError: false,
            error: null,
          } as unknown as UseQueryResult<
            GetFleetAvailabilityTrendsResponse,
            Error
          >;
        }
        return {
          data: mockOverallData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'By Model' }));

    expect(screen.getByText('SELECT SERIES TO DISPLAY:')).toBeInTheDocument();
    expect(screen.getByLabelText('brya')).toBeChecked();
    expect(screen.getByLabelText('volteer')).toBeChecked();
    expect(isPlotted('brya')).toBe(true);
    expect(isPlotted('volteer')).toBe(true);

    // Toggle off brya
    fireEvent.click(screen.getByLabelText('brya'));
    expect(screen.getByLabelText('brya')).not.toBeChecked();
    expect(isPlotted('brya')).toBe(false);
    expect(isPlotted('volteer')).toBe(true);
  });

  it('switches to By Pool tab and displays pool series', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((req) => {
        if (req.grouping === TrendlineGrouping.GROUP_BY_POOL) {
          return {
            data: mockPoolData,
            isLoading: false,
            isError: false,
            error: null,
          } as unknown as UseQueryResult<
            GetFleetAvailabilityTrendsResponse,
            Error
          >;
        }
        return {
          data: mockOverallData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'By Pool' }));

    expect(
      screen.getByText('Historical health percentage trendlines across pools.'),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('DUT_POOL_QUOTA')).toBeChecked();
    expect(isPlotted('DUT_POOL_QUOTA')).toBe(true);
  });

  it('shows tooltip on mouse hover over the chart', () => {
    giveChartASize();
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: mockOverallData,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    const { container } = render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    const surface = container.querySelector('.recharts-wrapper')!;
    fireEvent.mouseMove(surface, { clientX: 120, clientY: 120 });

    const tooltip = screen.getByTestId('trends-chart-tooltip');
    expect(within(tooltip).getByText('Overall')).toBeInTheDocument();
    expect(within(tooltip).getByText(/92\.5% Health/)).toBeInTheDocument();
    // Verify dev ratio is omitted from compact view
    expect(within(tooltip).queryByText(/ready/)).not.toBeInTheDocument();

    fireEvent.mouseLeave(surface);
    expect(
      screen.queryByTestId('trends-chart-tooltip'),
    ).not.toBeInTheDocument();
  });

  it('omits series from the tooltip that have no reading in that bucket', () => {
    giveChartASize();
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((req) => {
        if (req.grouping === TrendlineGrouping.GROUP_BY_MODEL) {
          return {
            data: {
              series: [
                // `brya` reports in both buckets, `volteer` only in the second.
                {
                  name: 'brya',
                  points: [
                    { timestamp: t0, value: 0.9 },
                    { timestamp: t1, value: 0.91 },
                  ],
                },
                { name: 'volteer', points: [{ timestamp: t1, value: 0.8 }] },
              ],
              metricType: TrendlineMetricType.AVAILABILITY,
            },
            isLoading: false,
            isError: false,
            error: null,
          } as unknown as UseQueryResult<
            GetFleetAvailabilityTrendsResponse,
            Error
          >;
        }
        return {
          data: mockOverallData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });

    const { container } = render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'By Model' }));

    // x=60 is just inside the plot area, so this lands in the first bucket:
    // the value axis is 48px wide and the chart's left margin is 0.
    const surface = container.querySelector('.recharts-wrapper')!;
    fireEvent.mouseMove(surface, { clientX: 60, clientY: 120 });

    const tooltip = screen.getByTestId('trends-chart-tooltip');
    expect(within(tooltip).getByText('brya')).toBeInTheDocument();
    expect(within(tooltip).queryByText('volteer')).not.toBeInTheDocument();
  });

  it('renders loading state', () => {
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
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('trends-loading')).toBeInTheDocument();
  });

  it('renders error state', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: undefined,
        isLoading: false,
        isError: true,
        error: new Error('RPC connection refused'),
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        /Failed to load historical trendlines: RPC connection refused/,
      ),
    ).toBeInTheDocument();
  });

  it('renders empty data message when no data is returned', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: { series: [], metricType: 'health' },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('No trend data available for the last 72 hours.'),
    ).toBeInTheDocument();
  });

  it('renders message when user deselects all series in multi-series view', () => {
    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockImplementation((req) => {
        if (req.grouping === TrendlineGrouping.GROUP_BY_MODEL) {
          return {
            data: mockModelData,
            isLoading: false,
            isError: false,
            error: null,
          } as unknown as UseQueryResult<
            GetFleetAvailabilityTrendsResponse,
            Error
          >;
        }
        return {
          data: mockOverallData,
          isLoading: false,
          isError: false,
          error: null,
        } as unknown as UseQueryResult<
          GetFleetAvailabilityTrendsResponse,
          Error
        >;
      });

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'By Model' }));
    // Deselect both brya and volteer
    fireEvent.click(screen.getByLabelText('brya'));
    fireEvent.click(screen.getByLabelText('volteer'));

    expect(
      screen.getByText(
        'No series selected. Please select at least one series from the list.',
      ),
    ).toBeInTheDocument();
  });

  it('renders correctly with a single data point without NaN errors', () => {
    const mockSinglePointData: GetFleetAvailabilityTrendsResponse = {
      series: [
        {
          name: 'Overall',
          points: [{ timestamp: '2026-09-14T10:00:00Z', value: 0.95 }],
        },
      ],
      metricType: TrendlineMetricType.HEALTH,
    };

    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: mockSinglePointData,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    render(
      <FakeContextProvider>
        <HistoricalAvailabilityTrendsChart />
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('trends-svg-chart')).toBeInTheDocument();
    // A lone reading has no line to draw, so it is marked with a dot.
    expect(isPlotted('Overall')).toBe(true);
    expect(screen.getByText(/9\/14 03:00 GMT-7/)).toBeInTheDocument();
  });

  describe('series colors', () => {
    const asQueryResult = (data: GetFleetAvailabilityTrendsResponse) =>
      ({
        data,
        isLoading: false,
        isError: false,
        error: null,
      }) as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >;

    /**
     * The color a series is drawn in: the stroke of its line, or, when a lone
     * reading leaves it with no line to draw, the fill of its dot. Dots are
     * stroked white, so reading `stroke` off an arbitrary element would report
     * the halo rather than the series color.
     */
    const colorOf = (name: string) => {
      const parts = partsOf(name);
      const curve = parts.find((el) => el.classList.contains('recharts-curve'));
      return curve?.getAttribute('stroke') ?? parts[0]?.getAttribute('fill');
    };

    it('keeps each series color when a refetch reorders the response', () => {
      const useTrends = jest
        .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
        .mockReturnValue(asQueryResult(mockModelData));

      const { rerender } = render(
        <FakeContextProvider>
          <HistoricalAvailabilityTrendsChart />
        </FakeContextProvider>,
      );
      fireEvent.click(screen.getByRole('button', { name: 'By Model' }));

      const bryaColor = colorOf('brya');
      const volteerColor = colorOf('volteer');
      expect(bryaColor).toBeTruthy();
      expect(bryaColor).not.toEqual(volteerColor);

      // A background refetch returns the same two series in the opposite
      // order, preceded by a model that was not previously present.
      useTrends.mockReturnValue(
        asQueryResult({
          series: [
            { name: 'kevin', points: [{ timestamp: t0, value: 0.5 }] },
            mockModelData.series[1],
            mockModelData.series[0],
          ],
          metricType: TrendlineMetricType.AVAILABILITY,
        }),
      );
      rerender(
        <FakeContextProvider>
          <HistoricalAvailabilityTrendsChart />
        </FakeContextProvider>,
      );

      expect(colorOf('brya')).toBe(bryaColor);
      expect(colorOf('volteer')).toBe(volteerColor);
      // The newly arrived series is not selected, so it is not plotted.
      expect(isPlotted('kevin')).toBe(false);
    });

    it('plots at most eight series, each in its own color', () => {
      const manyModels = Array.from({ length: 10 }, (_, i) => ({
        name: `model-${i}`,
        points: [
          { timestamp: t0, value: 0.9 },
          { timestamp: t1, value: 0.95 },
        ],
      }));
      jest
        .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
        .mockReturnValue(
          asQueryResult({
            series: manyModels,
            metricType: TrendlineMetricType.AVAILABILITY,
          }),
        );

      render(
        <FakeContextProvider>
          <HistoricalAvailabilityTrendsChart />
        </FakeContextProvider>,
      );
      fireEvent.click(screen.getByRole('button', { name: 'By Model' }));

      // Five are selected by default, leaving room for three more.
      expect(screen.getByLabelText('model-4')).toBeChecked();
      expect(screen.getByLabelText('model-5')).not.toBeChecked();
      expect(screen.getByLabelText('model-5')).toBeEnabled();

      fireEvent.click(screen.getByLabelText('model-5'));
      fireEvent.click(screen.getByLabelText('model-6'));
      fireEvent.click(screen.getByLabelText('model-7'));

      const plottedColors = manyModels
        .slice(0, 8)
        .map((series) => colorOf(series.name));
      expect(plottedColors.every((color) => color)).toBe(true);
      expect(new Set(plottedColors).size).toBe(8);

      // The palette is exhausted, so the remaining models cannot be added.
      expect(screen.getByText(/the maximum/)).toBeInTheDocument();
      expect(screen.getByLabelText('model-8')).toBeDisabled();

      // Clearing one frees its color for another series.
      fireEvent.click(screen.getByLabelText('model-0'));
      expect(screen.getByLabelText('model-8')).toBeEnabled();
      expect(isPlotted('model-0')).toBe(false);
    });
  });
});
