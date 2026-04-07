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

import { fireEvent, render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

import { FilterEditor, TimeSeriesChart } from '@/crystal_ball/components';
import { Column } from '@/crystal_ball/constants';
import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks';
import {
  createMockErrorResult,
  createMockPendingResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import {
  FetchDashboardWidgetDataResponse,
  MeasurementFilterColumn_ColumnDataType,
  MeasurementFilterColumn_FilterScope,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ChartWidget } from './chart_widget';

// Mock MUI components used
jest.mock('@mui/material', () => ({
  ...jest.requireActual('@mui/material'),
  CircularProgress: () => (
    <div data-testid="circular-progress">CircularProgress</div>
  ),
  Alert: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="alert">Alert: {children}</div>
  ),
  Typography: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="typography">{children}</div>
  ),
  Box: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="box">{children}</div>
  ),
}));

jest.mock('@/crystal_ball/hooks');
const mockUseFetchDashboardWidgetData = jest.mocked(
  useFetchDashboardWidgetData,
);

jest.mock('@/crystal_ball/components', () => ({
  ChartSeriesEditor: jest.fn(() => (
    <div data-testid="chart-series-editor"></div>
  )),
  FilterEditor: jest.fn(() => <div data-testid="filter-editor"></div>),
  TimeSeriesChart: jest.fn(({ chartTitle }) => (
    <div data-testid="time-series-chart">TimeSeriesChart: {chartTitle}</div>
  )),
  ChartWidgetToolbar: jest.fn(({ onChartTypeChange }) => (
    <div data-testid="chart-widget-toolbar">
      <button
        title="Scatter Plot"
        onClick={() =>
          onChartTypeChange(PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION)
        }
      />
      <button
        title="Line Chart"
        onClick={() =>
          onChartTypeChange(PerfChartWidget_ChartType.MULTI_METRIC_CHART)
        }
      />
    </div>
  )),
}));
const MockTimeSeriesChart = jest.mocked(TimeSeriesChart);

const baseWidget: PerfChartWidget = PerfChartWidget.fromPartial({
  dataSpecId: 'mockspec',
  displayName: 'Test Chart',
  series: [{ metricField: 'metric1' }, { metricField: 'metric2' }],
  filters: [
    {
      column: Column.ATP_TEST_NAME,
      textInput: { defaultValue: { values: ['test-value'] } },
    },
  ],
});

describe('ChartWidget', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Default mock implementations

    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'x',
          yAxisDataKey: 'y',
          lines: [],
        },
      }),
    );
    MockTimeSeriesChart.mockClear();
  });

  it('should display loading state', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(createMockPendingResult());
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(screen.getByTestId('circular-progress')).toBeInTheDocument();
  });

  it('should display error state', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockErrorResult(new Error('Failed to fetch')),
    );
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(screen.getByTestId('alert')).toHaveTextContent(
      `${COMMON_MESSAGES.ERROR_FETCHING_MEASUREMENTS}Failed to fetch`,
    );
  });

  it('should display no data message when response is empty', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'x',
          yAxisDataKey: 'y',
          lines: [],
        },
      }),
    );
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(screen.getByText(COMMON_MESSAGES.NO_DATA_FOUND)).toBeInTheDocument();
  });

  it('should display required message when atp_test_name filter is missing', () => {
    const widgetWithoutFilters = PerfChartWidget.fromPartial({
      ...baseWidget,
      filters: [],
    });
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'x',
          yAxisDataKey: 'y',
          lines: [],
        },
      }),
    );
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={widgetWithoutFilters} // No filters
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(
      screen.getByText(COMMON_MESSAGES.ATP_TEST_NAME_REQUIRED),
    ).toBeInTheDocument();
  });

  it('should NOT display required message when atp_test_name filter is present in series', () => {
    const widgetWithSeriesFilter = PerfChartWidget.fromPartial({
      ...baseWidget,
      filters: [],
      series: [
        {
          metricField: 'metric1',
          filters: [
            {
              column: Column.ATP_TEST_NAME,
              textInput: { defaultValue: { values: ['test-value'] } },
            },
          ],
        },
      ],
    });
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'x',
          yAxisDataKey: 'y',
          lines: [],
        },
      }),
    );
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={widgetWithSeriesFilter}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(
      screen.queryByText(COMMON_MESSAGES.ATP_TEST_NAME_REQUIRED),
    ).not.toBeInTheDocument();
  });

  it('should display no data message when series have no data points', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'x',
          yAxisDataKey: 'y',
          lines: [
            {
              seriesId: 's1',
              legendLabel: 'l1',
              dataPoints: [], // Empty points
              metricField: 'metric1',
            },
          ],
        },
      }),
    );
    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );
    expect(screen.getByText(COMMON_MESSAGES.NO_DATA_FOUND)).toBeInTheDocument();
  });

  it('should render TimeSeriesChart with transformed data', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'timestamp',
          yAxisDataKey: 'value',
          lines: [
            {
              seriesId: 's1',
              legendLabel: 'metric1',
              dataPoints: [
                { timestamp: 1000, value: 10 },
                { timestamp: 2000, value: 20 },
              ],
              metricField: 'metric1',
            },
          ],
        },
      }),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    expect(screen.getByTestId('time-series-chart')).toBeInTheDocument();
    expect(MockTimeSeriesChart).toHaveBeenCalledTimes(1);
    expect(MockTimeSeriesChart.mock.calls[0][0].series[0].data).toEqual([
      { x: 1000, y: 10, count: 1 },
      { x: 2000, y: 20, count: 1 },
    ]);
  });

  it('should pass isLoadingFilterColumns down to FilterEditor', () => {
    const MockFilterEditor = jest.mocked(FilterEditor);
    MockFilterEditor.mockClear();

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[
          {
            column: 'test_name',
            primary: true,
            dataType: MeasurementFilterColumn_ColumnDataType.STRING,
            sampleValues: [],
            isMetricKey: false,
            applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
          },
        ]}
        isLoadingFilterColumns={true}
      />,
    );

    expect(MockFilterEditor).toHaveBeenCalledTimes(1);
    expect(MockFilterEditor.mock.calls[0][0]).toMatchObject({
      isLoadingColumns: true,
      availableColumns: [
        {
          column: 'test_name',
          primary: true,
          dataType: MeasurementFilterColumn_ColumnDataType.STRING,
        },
      ],
    });
  });

  it('should pass value xAxisType to TimeSeriesChart when xAxisDataKey is Column.BUILD_ID', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: Column.BUILD_ID,
          yAxisDataKey: 'value',
          lines: [
            {
              seriesId: 's1',
              legendLabel: 'metric1',
              dataPoints: [
                {
                  timestamp: 1000,
                  value: 10,
                  [Column.BUILD_ID]: 12345,
                },
              ],
              metricField: 'metric1',
            },
          ],
        },
      }),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    expect(MockTimeSeriesChart.mock.calls[0][0]).toMatchObject({
      xAxisType: 'value',
    });
  });

  it('should render TimeSeriesChart with multiple series', () => {
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult({
        widgetId: 'w1',
        multiMetricChartData: {
          xAxisDataKey: 'timestamp',
          yAxisDataKey: 'value',
          lines: [
            {
              seriesId: 's1',
              legendLabel: 'metric1',
              dataPoints: [{ timestamp: 1000, value: 10 }],
              metricField: 'metric1',
            },
            {
              seriesId: 's2',
              legendLabel: 'metric2',
              dataPoints: [{ timestamp: 2000, value: 20 }],
              metricField: 'metric2',
            },
          ],
        },
      }),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    expect(MockTimeSeriesChart.mock.calls[0][0].series).toHaveLength(2);
    expect(MockTimeSeriesChart.mock.calls[0][0].series).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: 'metric1' }),
        expect.objectContaining({ name: 'metric2' }),
      ]),
    );
  });

  it('should render TimeSeriesChart with scatter plot for invocation distribution', () => {
    const distributionWidget = PerfChartWidget.fromPartial({
      ...baseWidget,
      chartType: PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    });
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          invocationDistributionData: {
            xAxisDataKey: 'timestamp',
            yAxisDataKey: 'value',
            points: [
              {
                legendLabel: 'dist1',
                seriesId: 's1',
                metricField: 'metric1',
                points: [
                  { timestamp: 1000, value: 10 },
                  { timestamp: 2000, value: 20 },
                ],
              },
            ],
          },
        }),
      ),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={distributionWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    expect(screen.getByTestId('time-series-chart')).toBeInTheDocument();
    expect(MockTimeSeriesChart).toHaveBeenCalledTimes(1);
    const lastCallProps = MockTimeSeriesChart.mock.lastCall![0];
    expect(lastCallProps.series[0].data).toEqual([
      { x: 1000, y: 10, count: 1 },
      { x: 2000, y: 20, count: 1 },
    ]);
    expect(lastCallProps.chartType).toBe('scatter');
  });

  it('should render TimeSeriesChart with scatter plot and parse string timestamps', () => {
    const distributionWidget = PerfChartWidget.fromPartial({
      ...baseWidget,
      chartType: PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    });
    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          invocationDistributionData: {
            xAxisDataKey: 'timestamp',
            yAxisDataKey: 'value',
            points: [
              {
                legendLabel: 'dist1',
                seriesId: 's1',
                metricField: 'metric1',
                points: [
                  { timestamp: '2026-04-04T10:00:00Z', value: 10 },
                  { timestamp: '2026-04-04T11:00:00Z', value: 20 },
                ],
              },
            ],
          },
        }),
      ),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={distributionWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    expect(screen.getByTestId('time-series-chart')).toBeInTheDocument();
    expect(MockTimeSeriesChart).toHaveBeenCalledTimes(1);
    const expectedTime1 = Date.parse('2026-04-04T10:00:00Z');
    const expectedTime2 = Date.parse('2026-04-04T11:00:00Z');
    const lastCallProps = MockTimeSeriesChart.mock.lastCall![0];
    expect(lastCallProps.series[0].data).toEqual([
      { x: expectedTime1, y: 10, count: 1 },
      { x: expectedTime2, y: 20, count: 1 },
    ]);
  });

  it('should call onUpdate with toggled chart type when toggle button is clicked', () => {
    const onUpdateMock = jest.fn();
    render(
      <ChartWidget
        onUpdate={onUpdateMock}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    const toggleButton = screen.getByTitle('Scatter Plot');
    expect(toggleButton).toBeInTheDocument();

    fireEvent.click(toggleButton);

    expect(onUpdateMock).toHaveBeenCalledTimes(1);
    expect(onUpdateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        chartType: PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
      }),
    );
  });

  it('should call onUpdate with toggled chart type when toggle button is clicked (from distribution)', () => {
    const onUpdateMock = jest.fn();
    const distributionWidget = PerfChartWidget.fromPartial({
      ...baseWidget,
      chartType: PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    });
    render(
      <ChartWidget
        onUpdate={onUpdateMock}
        widget={distributionWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
      />,
    );

    const toggleButton = screen.getByTitle('Line Chart');
    expect(toggleButton).toBeInTheDocument();

    fireEvent.click(toggleButton);

    expect(onUpdateMock).toHaveBeenCalledTimes(1);
    expect(onUpdateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        chartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
      }),
    );
  });

  it('should calculate xAxisBounds correctly from global filters and data', () => {
    const mockFilters = [
      PerfFilter.fromPartial({
        column: 'TIMESTAMP',
        range: {
          defaultValue: {
            filterOperator: PerfFilterDefault_FilterOperator.BETWEEN,
            values: ['2026-04-04T09:00:00Z', '2026-04-04T12:00:00Z'],
          },
        },
      }),
    ];

    mockUseFetchDashboardWidgetData.mockReturnValue(
      createMockQueryResult(
        FetchDashboardWidgetDataResponse.fromPartial({
          widgetId: 'w1',
          multiMetricChartData: {
            xAxisDataKey: 'timestamp',
            yAxisDataKey: 'value',
            lines: [
              {
                legendLabel: 'series1',
                dataPoints: [
                  { timestamp: '2026-04-04T10:00:00Z', value: 10 },
                  { timestamp: '2026-04-04T11:00:00Z', value: 20 },
                ],
              },
            ],
          },
        }),
      ),
    );

    render(
      <ChartWidget
        onUpdate={jest.fn()}
        widget={baseWidget}
        dashboardName="dashboardStates/d1"
        widgetId="w1"
        filterColumns={[]}
        globalFilters={mockFilters}
      />,
    );

    expect(MockTimeSeriesChart).toHaveBeenCalledTimes(1);
    const lastCallProps = MockTimeSeriesChart.mock.lastCall![0];

    const expectedTimeStart = Date.parse('2026-04-04T09:00:00Z');
    const expectedTimeEnd = Date.parse('2026-04-04T12:00:00Z');

    expect(lastCallProps.xAxisMin).toBe(expectedTimeStart);
    expect(lastCallProps.xAxisMax).toBe(expectedTimeEnd);
  });
});
