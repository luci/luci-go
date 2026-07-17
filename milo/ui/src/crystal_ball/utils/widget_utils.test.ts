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
  calculateChange,
  calculateTooltipPosition,
  dataPointsToData,
  formatChange,
  getSafeChartType,
  getTrendInfo,
  isDataPointsValid,
  isLowerIsBetter,
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

describe('isLowerIsBetter', () => {
  test('returns true for latency and duration metric keywords', () => {
    expect(isLowerIsBetter('duration_ms')).toBe(true);
    expect(isLowerIsBetter('proc_meminfo_rss_bytes')).toBe(true);
  });

  test('returns false for neutral or other keywords', () => {
    expect(isLowerIsBetter('fps')).toBe(false);
    expect(isLowerIsBetter('score')).toBe(false);
    expect(isLowerIsBetter(undefined)).toBe(false);
  });

  test('returns false for exceptions that contain keywords but where higher is better', () => {
    expect(isLowerIsBetter('uptime')).toBe(false);
    expect(isLowerIsBetter('uptime_s')).toBe(false);
    expect(isLowerIsBetter('free_memory')).toBe(false);
    expect(isLowerIsBetter('available_bytes')).toBe(false);
  });
});

describe('calculateChange', () => {
  test('calculates standard absolute and percentage change', () => {
    const { diff, pctChange } = calculateChange(100, 125);
    expect(diff).toBe(25);
    expect(pctChange).toBe(25);
  });

  test('handles baseline values equal to zero', () => {
    const { diff, pctChange } = calculateChange(0, 50);
    expect(diff).toBe(50);
    expect(pctChange).toBe(0);
  });
});

describe('formatChange', () => {
  test('formats positive change with plus prefix', () => {
    expect(formatChange(25.45, 12.34)).toBe('+25.45 (+12.3%)');
  });

  test('formats negative change with minus prefix', () => {
    expect(formatChange(-10.2, -5.6)).toBe('-10.2 (-5.6%)');
  });
});

describe('getTrendInfo', () => {
  test('identifies latency increases as regressions', () => {
    const info = getTrendInfo(15, 'duration_ms');
    expect(info.trend).toBe('up');
    expect(info.color).toBe('error.main');
    expect(info.isRegression).toBe(true);
  });

  test('identifies latency decreases as improvements', () => {
    const info = getTrendInfo(-10, 'duration_ms');
    expect(info.trend).toBe('down');
    expect(info.color).toBe('success.main');
    expect(info.isRegression).toBe(false);
  });

  test('identifies score increases as improvements', () => {
    const info = getTrendInfo(5, 'score');
    expect(info.trend).toBe('up');
    expect(info.color).toBe('success.main');
    expect(info.isRegression).toBe(false);
  });

  test('identifies score decreases as regressions', () => {
    const info = getTrendInfo(-5, 'score');
    expect(info.trend).toBe('down');
    expect(info.color).toBe('error.main');
    expect(info.isRegression).toBe(true);
  });

  test('handles flat trend states', () => {
    const info = getTrendInfo(0, 'duration_ms');
    expect(info.trend).toBe('flat');
    expect(info.color).toBe('text.secondary');
    expect(info.isRegression).toBe(false);
  });
});

describe('calculateTooltipPosition', () => {
  const offset = 5;

  test('uses simple cursor offset when size details are not available', () => {
    const result = calculateTooltipPosition([100, 150], undefined, offset);
    expect(result).toEqual([105, 155]);
  });

  test('positions tooltip to the right and below by default', () => {
    const size = {
      contentSize: [120, 80] as [number, number],
      viewSize: [500, 400] as [number, number],
    };
    // Cursor is at [100, 100] - rendering to right/below fits inside [500, 400]
    const result = calculateTooltipPosition([100, 100], size, offset);
    expect(result).toEqual([105, 105]);
  });

  test('flips tooltip horizontally to the left of cursor when right boundary overflows', () => {
    const size = {
      contentSize: [120, 80] as [number, number],
      viewSize: [500, 400] as [number, number],
    };
    // Cursor is at [400, 100] - rendering to right (400+120+5 = 525) overflows 500.
    // Flipped coordinate posX should be 400 - 120 - 5 = 275.
    const result = calculateTooltipPosition([400, 100], size, offset);
    expect(result).toEqual([275, 105]);
  });

  test('shifts/flips tooltip vertically upwards when bottom boundary overflows', () => {
    const size = {
      contentSize: [120, 80] as [number, number],
      viewSize: [500, 400] as [number, number],
    };
    // Cursor is at [100, 350] - rendering below (350+80+5 = 435) overflows 400.
    // posY should shift up to 350 - 80 - 5 = 265.
    const result = calculateTooltipPosition([100, 350], size, offset);
    expect(result).toEqual([105, 265]);
  });

  test('handles dual boundary collision (flips both horizontally and vertically)', () => {
    const size = {
      contentSize: [120, 80] as [number, number],
      viewSize: [500, 400] as [number, number],
    };
    // Cursor is at [450, 360] - both overflow boundaries.
    // posX: 450 - 120 - 5 = 325
    // posY: 360 - 80 - 5 = 275
    const result = calculateTooltipPosition([450, 360], size, offset);
    expect(result).toEqual([325, 275]);
  });
});
