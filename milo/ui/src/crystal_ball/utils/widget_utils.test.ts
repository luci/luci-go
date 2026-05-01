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

import {
  PerfChartSeries_PerfAggregationFunction,
  PerfChartWidget,
  PerfChartWidget_ChartType,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import {
  dataPointsToData,
  getSafeChartType,
  isDataPointsValid,
  sanitizeChartWidget,
} from './widget_utils';

describe('sanitizeChartWidget', () => {
  test('sets default aggregation if unspecified or 0', () => {
    const rawWidget = PerfChartWidget.fromPartial({
      chartType: PerfChartWidget_ChartType.MULTI_METRIC_CHART,
      series: [
        {
          metricField: 'metric_a',
          aggregation: 0,
        },
      ],
    });

    const sanitized = sanitizeChartWidget(rawWidget);
    expect(sanitized.series![0].aggregation).toBe(
      PerfChartSeries_PerfAggregationFunction.MEAN,
    );
  });

  test('preserves existing valid chart types', () => {
    const rawWidget = PerfChartWidget.fromPartial({
      chartType: PerfChartWidget_ChartType.BREAKDOWN_TABLE,
    });

    const sanitized = sanitizeChartWidget(rawWidget);
    expect(sanitized.chartType).toBe(PerfChartWidget_ChartType.BREAKDOWN_TABLE);
  });
});

describe('getSafeChartType', () => {
  it('should return valid chart type from string', () => {
    expect(getSafeChartType('MULTI_METRIC_CHART')).toBe(
      PerfChartWidget_ChartType.MULTI_METRIC_CHART,
    );
  });

  it('should return unspecified for invalid value', () => {
    expect(getSafeChartType('INVALID')).toBe(
      PerfChartWidget_ChartType.CHART_TYPE_UNSPECIFIED,
    );
    expect(getSafeChartType(null)).toBe(
      PerfChartWidget_ChartType.CHART_TYPE_UNSPECIFIED,
    );
  });
});

describe('isDataPointsValid', () => {
  it('should return true for valid points', () => {
    const points = [{ x: 10, y: 20 }];
    expect(isDataPointsValid(points, 'x', 'y')).toBe(true);
  });

  it('should return true for string values that are numbers', () => {
    const points = [{ x: '10', y: '20' }];
    expect(isDataPointsValid(points, 'x', 'y')).toBe(true);
  });

  it('should return false for invalid types', () => {
    const points = [{ x: true, y: 20 }];
    expect(isDataPointsValid(points, 'x', 'y')).toBe(false);
  });
});

describe('dataPointsToData', () => {
  it('should transform numeric points', () => {
    const points = [{ x: 10, y: 20 }];
    const data = dataPointsToData(points, 'x', 'y');
    expect(data).toEqual([{ x: 10, y: 20, count: 1, point: points[0] }]);
  });

  it('should parse string numbers', () => {
    const points = [{ x: '10', y: '20' }];
    const data = dataPointsToData(points, 'x', 'y');
    expect(data).toEqual([{ x: 10, y: 20, count: 1, point: points[0] }]);
  });

  it('should parse date strings', () => {
    const points = [{ x: '2026-04-04T10:00:00Z', y: 20 }];
    const data = dataPointsToData(points, 'x', 'y');
    const expectedTime = Date.parse('2026-04-04T10:00:00Z');
    expect(data).toEqual([
      { x: expectedTime, y: 20, count: 1, point: points[0] },
    ]);
  });

  it('should fallback to 0 for invalid date strings', () => {
    const points = [{ x: 'invalid-date', y: 20 }];
    const data = dataPointsToData(points, 'x', 'y');
    expect(data).toEqual([{ x: 0, y: 20, count: 1, point: points[0] }]);
  });
});
