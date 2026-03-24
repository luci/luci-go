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

import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

import { FilterEditor } from '@/crystal_ball/components';
import { TimeSeriesChart } from '@/crystal_ball/components';
import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks';
import {
  createMockErrorResult,
  createMockPendingResult,
  createMockQueryResult,
} from '@/crystal_ball/tests';
import {
  MeasurementFilterColumn_ColumnDataType,
  MeasurementFilterColumn_FilterScope,
  PerfChartWidget,
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
}));
const MockTimeSeriesChart = jest.mocked(TimeSeriesChart);

const baseWidget: PerfChartWidget = PerfChartWidget.fromPartial({
  dataSpecId: 'mockspec',
  displayName: 'Test Chart',
  series: [{ metricField: 'metric1' }, { metricField: 'metric2' }],
  filters: [],
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
    expect(screen.getByTestId('typography')).toHaveTextContent(
      COMMON_MESSAGES.NO_DATA_FOUND,
    );
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
    expect(screen.getByTestId('typography')).toHaveTextContent(
      COMMON_MESSAGES.NO_DATA_FOUND,
    );
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
      [1000, 10],
      [2000, 20],
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
});
