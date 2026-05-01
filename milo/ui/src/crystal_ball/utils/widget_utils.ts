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

import { NUM_AGGREGATED_ROWS } from '@/crystal_ball/constants';
import {
  PerfChartWidget,
  PerfChartWidget_ChartType,
  perfChartWidget_ChartTypeFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Returns a safe chart type from the given chart type.
 */
export function getSafeChartType(
  chartType: string | number | undefined | null,
): PerfChartWidget_ChartType {
  if (
    (typeof chartType === 'string' || typeof chartType === 'number') &&
    chartType in PerfChartWidget_ChartType
  ) {
    return perfChartWidget_ChartTypeFromJSON(chartType);
  }
  return PerfChartWidget_ChartType.CHART_TYPE_UNSPECIFIED;
}

/**
 * Sanitizes a PerfChartWidget by ensuring it has default group-by options
 * when x-axis column is unspecified.
 */
export function sanitizeChartWidget(chart: PerfChartWidget): PerfChartWidget {
  return PerfChartWidget.fromPartial({
    ...chart,
    series: chart.series?.map((s) => ({
      ...s,
      aggregation:
        s.aggregation === 0 || s.aggregation === undefined ? 1 : s.aggregation,
    })),
    chartType: getSafeChartType(chart.chartType),
  });
}

/**
 * Validates that all data points have valid numeric or string values for specified X and Y axis keys.
 */
export const isDataPointsValid = (
  dataPoints: readonly { readonly [key: string]: unknown }[],
  xAxisDataKey: string,
  yAxisDataKey: string,
): dataPoints is { [axisDataKey: string]: number | string }[] => {
  if (dataPoints === null || dataPoints === undefined) return false;

  return (
    Array.isArray(dataPoints) &&
    dataPoints.every(
      (dataPoint) =>
        dataPoint !== null &&
        dataPoint !== undefined &&
        (typeof dataPoint[xAxisDataKey] === 'number' ||
          typeof dataPoint[xAxisDataKey] === 'string') &&
        (typeof dataPoint[yAxisDataKey] === 'number' ||
          typeof dataPoint[yAxisDataKey] === 'string'),
    )
  );
};

/**
 * Transforms data points into a plain numeric array format [time, value] for TimeSeriesChart.
 */
export function dataPointsToData(
  dataPoints: { [axisDataKey: string]: number | string }[],
  xAxisDataKey: string,
  yAxisDataKey: string,
): Array<{
  x: number;
  y: number;
  count: number;
  point?: Record<string, unknown>;
}> {
  return dataPoints.map((pt) => {
    const xValue = pt[xAxisDataKey];
    let x: number;

    if (typeof xValue === 'number') {
      x = xValue;
    } else if (typeof xValue === 'string') {
      if (!isNaN(Number(xValue)) && isFinite(Number(xValue))) {
        x = Number(xValue);
      } else {
        const parsedDate = Date.parse(xValue);
        x = isNaN(parsedDate) ? 0 : parsedDate;
      }
    } else {
      x = 0; // Fallback for safety
    }

    const yValue = pt[yAxisDataKey];
    const y =
      typeof yValue === 'string'
        ? parseFloat(yValue)
        : typeof yValue === 'number'
          ? yValue
          : 0;
    const countVal = pt[NUM_AGGREGATED_ROWS];
    const count = typeof countVal === 'number' ? countVal : 1;

    return { x, y, count, point: pt };
  });
}
