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
import { DateTime } from 'luxon';

import * as timeUtils from '@/common/components/time_range_selector/time_range_selector_utils';
import * as components from '@/crystal_ball/components';
import * as useSearchMeasurementsHook from '@/crystal_ball/hooks/use_android_perf_api';
import {
  PerfChartWidget,
  MeasurementRow,
  SearchMeasurementsRequest,
} from '@/crystal_ball/types';
import * as useSyncedSearchParamsHook from '@/generic_libs/hooks/synced_search_params';

import { ChartWidget, transformDataForChart } from './chart_widget';

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

// Mock custom hooks and utils
jest.mock('@/generic_libs/hooks/synced_search_params');
const mockUseSyncedSearchParams =
  useSyncedSearchParamsHook.useSyncedSearchParams as jest.Mock;

jest.mock('@/crystal_ball/hooks/use_android_perf_api');
const mockUseSearchMeasurements =
  useSearchMeasurementsHook.useSearchMeasurements as jest.Mock;

jest.mock('@/common/components/time_range_selector/time_range_selector_utils');
const mockGetAbsoluteStartEndTime =
  timeUtils.getAbsoluteStartEndTime as jest.Mock;

jest.mock('@/crystal_ball/components', () => ({
  TimeSeriesChart: jest.fn(({ chartTitle }) => (
    <div data-testid="time-series-chart">TimeSeriesChart: {chartTitle}</div>
  )),
}));
const MockTimeSeriesChart = components.TimeSeriesChart as jest.Mock;

const baseWidget: PerfChartWidget = {
  dataSpecId: 'mockspec',
  displayName: 'Test Chart',
  series: [{ metricField: 'metric1' }, { metricField: 'metric2' }],
  filters: [],
};

const now = DateTime.local(2026, 3, 4, 18, 58, 53);

describe('ChartWidget', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Default mock implementations
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams(),
      jest.fn(),
    ]);
    mockGetAbsoluteStartEndTime.mockReturnValue({
      startTime: now.minus({ days: 1 }),
      endTime: now,
    });
    mockUseSearchMeasurements.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
      error: null,
    });
    MockTimeSeriesChart.mockClear();
  });

  it('should display loading state', () => {
    mockUseSearchMeasurements.mockReturnValue({ isLoading: true });
    render(<ChartWidget widget={baseWidget} />);
    expect(screen.getByTestId('circular-progress')).toBeInTheDocument();
  });

  it('should display error state', () => {
    mockUseSearchMeasurements.mockReturnValue({
      isError: true,
      error: { message: 'Failed to fetch' },
    });
    render(<ChartWidget widget={baseWidget} />);
    expect(screen.getByTestId('alert')).toHaveTextContent(
      'Error fetching chart data: Failed to fetch',
    );
  });

  it('should display no data message when response is empty', () => {
    mockUseSearchMeasurements.mockReturnValue({ data: { rows: [] } });
    render(<ChartWidget widget={baseWidget} />);
    expect(screen.getByTestId('typography')).toHaveTextContent(
      'No data found for the given parameters.',
    );
  });

  it('should display no data message when series have no data points', () => {
    mockUseSearchMeasurements.mockReturnValue({
      data: {
        rows: [
          // Data for a different metric
          {
            buildCreateTime: now.minus({ hours: 1 }).toISO(),
            metricKey: 'other_metric',
            value: 10,
            buildId: '1',
          },
        ],
      },
    });
    render(<ChartWidget widget={baseWidget} />);
    expect(screen.getByTestId('typography')).toHaveTextContent(
      'No data found for the given parameters.',
    );
  });

  it('should render TimeSeriesChart with transformed data', () => {
    const rows: MeasurementRow[] = [
      {
        buildCreateTime: now.minus({ hours: 2 }).toISO(),
        metricKey: 'metric1',
        value: 10,
        buildId: '1',
      },
      {
        buildCreateTime: now.minus({ hours: 2 }).toISO(),
        metricKey: 'metric1',
        value: 20,
        buildId: '2',
      }, // Same time, new build
      {
        buildCreateTime: now.minus({ hours: 1 }).toISO(),
        metricKey: 'metric1',
        value: 16,
        buildId: '3',
      },
      {
        buildCreateTime: now.minus({ hours: 2 }).toISO(),
        metricKey: 'metric2',
        value: 100,
        buildId: '1',
      },
    ];
    mockUseSearchMeasurements.mockReturnValue({ data: { rows } });

    render(<ChartWidget widget={baseWidget} />);

    expect(screen.getByTestId('time-series-chart')).toBeInTheDocument();
    expect(MockTimeSeriesChart).toHaveBeenCalledTimes(1);
  });

  it('should build search request based on widget series and time params', () => {
    mockUseSyncedSearchParams.mockReturnValue([
      new URLSearchParams('time_option=last_7_days'),
      jest.fn(),
    ]);
    const startTime = now.minus({ days: 7 });
    const endTime = now;
    mockGetAbsoluteStartEndTime.mockReturnValue({ startTime, endTime });

    render(<ChartWidget widget={baseWidget} />);

    expect(mockGetAbsoluteStartEndTime).toHaveBeenCalledWith(
      expect.any(URLSearchParams),
      expect.any(DateTime),
    );

    const expectedRequest: SearchMeasurementsRequest = {
      metricKeys: ['metric1', 'metric2'],
      buildCreateStartTime: {
        seconds: startTime.toUnixInteger().toString(),
        nanos: 0,
      },
      buildCreateEndTime: {
        seconds: endTime.toUnixInteger().toString(),
        nanos: 0,
      },
    };
    expect(mockUseSearchMeasurements).toHaveBeenCalledWith(expectedRequest, {
      enabled: true,
    });
  });

  it('should apply filters to the search request', () => {
    const widgetWithFilters: PerfChartWidget = {
      ...baseWidget,
      filters: [
        {
          id: 'id1',
          dataSpecId: 'mockspec',
          column: 'test_name',
          textInput: {
            defaultValue: { values: ['MyTest'], filterOperator: 'CONTAINS' },
          },
        },
        {
          id: 'id2',
          dataSpecId: 'mockspec',
          column: 'build_target',
          textInput: { defaultValue: { values: ['targetA'] } },
        }, // EQUAL by default
        {
          id: 'id3',
          dataSpecId: 'mockspec',
          column: 'atp_test_name',
          textInput: {
            defaultValue: {
              values: ['AtpTest'],
              filterOperator: 'STARTS_WITH',
            },
          },
        },
        {
          id: 'id4',
          dataSpecId: 'mockspec',
          column: 'build_branch',
          textInput: { defaultValue: { values: ['main'] } },
        },
      ],
    };

    const startTime = now.minus({ days: 1 });
    const endTime = now;
    mockGetAbsoluteStartEndTime.mockReturnValue({ startTime, endTime });

    render(<ChartWidget widget={widgetWithFilters} />);

    const expectedRequest: SearchMeasurementsRequest = {
      metricKeys: ['metric1', 'metric2'],
      buildCreateStartTime: {
        seconds: startTime.toUnixInteger().toString(),
        nanos: 0,
      },
      buildCreateEndTime: {
        seconds: endTime.toUnixInteger().toString(),
        nanos: 0,
      },
      testNameFilter: '%MyTest%',
      buildTarget: 'targetA',
      atpTestNameFilter: 'AtpTest%',
      buildBranch: 'main',
    };
    expect(mockUseSearchMeasurements).toHaveBeenCalledWith(expectedRequest, {
      enabled: true,
    });
  });

  it('should handle undefined series in widget', () => {
    const widgetNoSeries: PerfChartWidget = {
      dataSpecId: 'mockspec',
      displayName: 'No Series Chart',
    };
    render(<ChartWidget widget={widgetNoSeries} />);
    // useSearchMeasurements should not be enabled
    expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
      expect.objectContaining({ metricKeys: [] }),
      { enabled: false },
    );
    expect(screen.getByTestId('typography')).toHaveTextContent(
      'No data found for the given parameters.',
    );
  });
});

describe('transformDataForChart', () => {
  const t1 = new Date('2026-03-04T10:00:00.000Z').toISOString();
  const t2 = new Date('2026-03-04T11:00:00.000Z').toISOString();

  const rows: MeasurementRow[] = [
    { buildCreateTime: t1, metricKey: 'm1', value: 10, buildId: 'b1' },
    { buildCreateTime: t1, metricKey: 'm1', value: 20, buildId: 'b2' }, // Same time, m1, different build
    { buildCreateTime: t2, metricKey: 'm1', value: 15, buildId: 'b3' },
    { buildCreateTime: t1, metricKey: 'm2', value: 100, buildId: 'b1' },
    { buildCreateTime: t2, metricKey: 'm2', value: 110, buildId: 'b3' },
    { buildCreateTime: t1, metricKey: 'm3', value: 99, buildId: 'b1' }, // Metric not requested
  ];

  it('should aggregate values by mean for each metric key and time', () => {
    const metricKeys = ['m1', 'm2'];
    const result = transformDataForChart(rows, metricKeys);

    expect(result).toHaveLength(2);

    // Check m1
    expect(result[0].name).toBe('m1');
    expect(result[0].data).toEqual([
      [new Date(t1).getTime(), 15], // (10 + 20) / 2
      [new Date(t2).getTime(), 15],
    ]);

    // Check m2
    expect(result[1].name).toBe('m2');
    expect(result[1].data).toEqual([
      [new Date(t1).getTime(), 100],
      [new Date(t2).getTime(), 110],
    ]);
  });

  it('should return empty data arrays for metrics with no data', () => {
    const metricKeys = ['m1', 'nonexistent'];
    const result = transformDataForChart(rows, metricKeys);

    expect(result).toHaveLength(2);
    expect(result[0].name).toBe('m1');
    expect(result[0].data.length).toBeGreaterThan(0);
    expect(result[1].name).toBe('nonexistent');
    expect(result[1].data).toEqual([]);
  });

  it('should generate distinct stroke colors', () => {
    const metricKeys = ['m1', 'm2', 'm3', 'm4'];
    const result = transformDataForChart(rows, metricKeys);
    expect(result[0].stroke).toMatch(/hsl\(\d+(\.\d+)?,\s*70%,\s*50%\)/);
    expect(result[1].stroke).toMatch(/hsl\(\d+(\.\d+)?,\s*70%,\s*50%\)/);
    expect(result[0].stroke).not.toBe(result[1].stroke);
  });

  it('should handle empty rows input', () => {
    const result = transformDataForChart([], ['m1']);
    expect(result).toEqual([
      { name: 'm1', data: [], stroke: expect.any(String) },
    ]);
  });
});
