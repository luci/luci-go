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
  PerfChartSeries,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  perfChartWidget_ChartTypeFromJSON,
  PerfDashboardContent,
  PerfFilter,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { generateColor } from './colors';

/**
 * Upper bound on palette lookups when searching for an unused color, to
 * guarantee termination even if every generated color is already taken.
 */
const MAX_PALETTE_LOOKUPS = 1000;

/**
 * Finds the matching series index in the widget's configured series list.
 * Matching precedence:
 * 1. Exact series ID match (s.id === seriesId)
 * 2. Parent ID match for expanded child series (e.g. "series-id_0" -> parent ID "series-id")
 * 3. Index fallback (seriesIndex)
 */
export function findMatchingSeriesIndex(
  seriesList?: readonly PerfChartSeries[],
  seriesId?: string,
  seriesIndex?: number,
): number {
  if (!seriesList || seriesList.length === 0) {
    return -1;
  }

  if (seriesId) {
    // 1. Exact ID match
    const exactIndex = seriesList.findIndex((s) => s.id === seriesId);
    if (exactIndex >= 0) {
      return exactIndex;
    }

    // 2. Child series ID match with numeric index suffix (e.g. "<parent_id>_<number>")
    const childMatch = seriesId.match(/^(.+)_\d+$/);
    if (childMatch) {
      const parentId = childMatch[1];
      const parentIndex = seriesList.findIndex((s) => s.id === parentId);
      if (parentIndex >= 0) {
        return parentIndex;
      }
    }
  }

  // 3. Index fallback
  if (
    seriesIndex !== undefined &&
    seriesIndex >= 0 &&
    seriesIndex < seriesList.length
  ) {
    return seriesIndex;
  }

  return -1;
}

/**
 * Finds the matching PerfChartSeries configuration in the widget's configured series list.
 */
export function findMatchingSeriesConfig(
  seriesList?: readonly PerfChartSeries[],
  seriesId?: string,
  seriesIndex?: number,
): PerfChartSeries | undefined {
  const index = findMatchingSeriesIndex(seriesList, seriesId, seriesIndex);
  return index >= 0 && seriesList ? seriesList[index] : undefined;
}

/**
 * Assigns a stroke color to each rendered chart line so that the chart,
 * tooltip and the CHART SERIES legend all agree.
 *
 * Rules:
 * 1. A line whose ID exactly matches a configured series uses that series'
 *    color.
 * 2. Otherwise, the expanded child line with the lowest index suffix (e.g.
 *    "<parent_id>_0") of a configured series inherits the parent's color, so
 *    the parent's legend dot always matches a visible line.
 * 3. All remaining lines (additional children, unmatched lines, or series
 *    without a configured color) get a distinct palette color that is not
 *    already used by any configured series or previously assigned line.
 *
 * Configured colors (rules 1 and 2) do not depend on the order in which the
 * backend returns lines.
 *
 * @param lineSeriesIds The series ID of each rendered line, in render order.
 * @param seriesList The widget's configured series.
 * @returns A stroke color for each entry in `lineSeriesIds`.
 */
export function assignSeriesStrokeColors(
  lineSeriesIds: readonly (string | undefined)[],
  seriesList?: readonly PerfChartSeries[],
): string[] {
  const usedColors = new Set<string>(
    (seriesList ?? [])
      .map((s) => s.color?.toLowerCase())
      .filter((c): c is string => !!c),
  );
  let paletteIndex = 0;
  const nextPaletteColor = (): string => {
    let color = generateColor(paletteIndex++);
    for (
      let lookups = 0;
      usedColors.has(color.toLowerCase()) && lookups < MAX_PALETTE_LOOKUPS;
      lookups++
    ) {
      color = generateColor(paletteIndex++);
    }
    usedColors.add(color.toLowerCase());
    return color;
  };

  const strokes: (string | undefined)[] = lineSeriesIds.map(() => undefined);
  const matchedIndices = lineSeriesIds.map((id) =>
    findMatchingSeriesIndex(seriesList, id),
  );
  const claimedSeries = new Set<number>();

  // Pass 1: exact ID matches always use their configured color.
  lineSeriesIds.forEach((id, i) => {
    const config =
      matchedIndices[i] >= 0 ? seriesList?.[matchedIndices[i]] : undefined;
    if (config?.color && id === config.id) {
      strokes[i] = config.color;
      claimedSeries.add(matchedIndices[i]);
    }
  });

  // Pass 2: for each series without an exact match, the child line with the
  // lowest numeric suffix inherits the series' color.
  const inheritingLine = new Map<number, { line: number; suffix: number }>();
  lineSeriesIds.forEach((id, i) => {
    const seriesIdx = matchedIndices[i];
    if (strokes[i] !== undefined || seriesIdx < 0) return;
    if (claimedSeries.has(seriesIdx) || !seriesList?.[seriesIdx]?.color) return;
    const suffixMatch = id?.match(/_(\d+)$/);
    const suffix = suffixMatch ? Number(suffixMatch[1]) : Infinity;
    const current = inheritingLine.get(seriesIdx);
    if (!current || suffix < current.suffix) {
      inheritingLine.set(seriesIdx, { line: i, suffix });
    }
  });
  inheritingLine.forEach(({ line }, seriesIdx) => {
    strokes[line] = seriesList?.[seriesIdx]?.color;
  });

  // Pass 3: everything else gets a distinct, unused palette color.
  return strokes.map((stroke) => stroke ?? nextPaletteColor());
}

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

/**
 * Sorts filters by ID to provide a stable order for query keys.
 */
function sortFilters(
  filters?: readonly PerfFilter[],
): PerfFilter[] | undefined {
  if (!filters) return undefined;
  return [...filters].sort((a, b) => a.id.localeCompare(b.id));
}

/**
 * Normalizes filters in a widget (both widget-level and series-level)
 * to avoid refetches when only the order changes.
 */
function normalizeWidgetFilters(widget: PerfChartWidget): PerfChartWidget {
  return PerfChartWidget.fromPartial({
    ...widget,
    filters: sortFilters(widget.filters),
    series: widget.series?.map((s) => ({
      ...s,
      filters: sortFilters(s.filters),
    })),
  });
}

/**
 * Normalizes filters in a request object to provide a stable order for query keys.
 * Supports FetchDashboardWidgetDataRequest and FetchWidgetRawSamplesRequest.
 */
export function normalizeRequestFilters<
  T extends { dashboardContent?: PerfDashboardContent },
>(request: T): T {
  if (!request.dashboardContent) return request;

  return {
    ...request,
    dashboardContent: {
      ...request.dashboardContent,
      globalFilters: sortFilters(request.dashboardContent.globalFilters) ?? [],
      widgets: request.dashboardContent.widgets?.map((w) => ({
        ...w,
        chart: w.chart ? normalizeWidgetFilters(w.chart) : undefined,
      })),
    },
  };
}

/**
 * Returns whether lower values are better for the given metric field name.
 */
export function isLowerIsBetter(metricField: string | undefined): boolean {
  if (!metricField) return false;

  const normalized = metricField.toLowerCase();

  // Exceptions where higher is better, even if they contain lowerIsBetter keywords.
  const higherIsBetterKeywords = [
    'uptime',
    'up_time',
    'free',
    'available',
    'efficiency',
  ];

  if (higherIsBetterKeywords.some((keyword) => normalized.includes(keyword))) {
    return false;
  }

  const lowerIsBetterKeywords = [
    'duration',
    'latency',
    'time',
    'bytes',
    'size',
    'memory',
    'pss',
    'rss',
    'alloc',
  ];

  return lowerIsBetterKeywords.some((keyword) => {
    return normalized.includes(keyword);
  });
}

/**
 * Determines polarity and returns trend state and MUI palette color tokens.
 */
export function getTrendInfo(
  netChange: number,
  metricField: string | undefined,
): {
  trend: 'up' | 'down' | 'flat';
  color: 'success.main' | 'error.main' | 'text.secondary';
  isRegression: boolean;
} {
  if (Math.abs(netChange) < 0.0001) {
    return {
      trend: 'flat',
      color: 'text.secondary',
      isRegression: false,
    };
  }

  const lowerIsBetter = isLowerIsBetter(metricField);
  const isPositive = netChange > 0;

  if (lowerIsBetter) {
    const isRegression = isPositive; // For latency, increase is bad (regression)
    return {
      trend: isPositive ? 'up' : 'down',
      color: isRegression ? 'error.main' : 'success.main',
      isRegression,
    };
  }

  const isRegression = !isPositive; // For score, decrease is bad (regression)
  return {
    trend: isPositive ? 'up' : 'down',
    color: isRegression ? 'error.main' : 'success.main',
    isRegression,
  };
}

export interface RegressionStats {
  diff: number;
  pctChange: number;
}

/**
 * Calculates absolute difference and percentage change between a baseline and a target value.
 */
export function calculateChange(
  baseline: number,
  target: number,
): RegressionStats {
  const diff = target - baseline;
  let pctChange = 0;
  if (baseline !== 0) {
    pctChange = (diff / baseline) * 100;
  }
  return { diff, pctChange };
}

/**
 * Formats absolute and percentage change into a clean standard string: "+1,234 (+12.3%)" or "-123 (-5.4%)".
 */
export function formatChange(diff: number, pctChange: number): string {
  let formattedDiff = diff.toLocaleString();
  if (diff > 0) {
    formattedDiff = `+${formattedDiff}`;
  }
  let formattedPct = `${pctChange.toFixed(1)}%`;
  if (pctChange > 0) {
    formattedPct = `+${formattedPct}`;
  }
  return `${formattedDiff} (${formattedPct})`;
}

export interface TooltipSizeInfo {
  contentSize: [number, number];
  viewSize: [number, number];
}

/**
 * Calculates the optimal absolute coordinates for the ECharts tooltip box.
 * If the tooltip would overflow the right or bottom chart boundaries, it flips
 * horizontally and/or shifts vertically to prevent clipping and cursor blockage.
 *
 * @param point - Bounding coordinate of the cursor relative to the container: [x, y]
 * @param size - Bounding box sizes of the tooltip content and chart viewport container
 * @param offsetPx - Horizontal/vertical spacing offset from the cursor
 * @returns Calculated coordinates [posX, posY]
 */
export function calculateTooltipPosition(
  point: [number, number],
  size: TooltipSizeInfo | undefined,
  offsetPx: number,
): [number, number] {
  const [x, y] = point;
  if (!size) {
    return [x + offsetPx, y + offsetPx];
  }

  const [tooltipW, tooltipH] = size.contentSize;
  const [chartW, chartH] = size.viewSize;

  const posX =
    x + tooltipW + offsetPx > chartW ? x - tooltipW - offsetPx : x + offsetPx;

  const posY =
    y + tooltipH + offsetPx > chartH ? y - tooltipH - offsetPx : y + offsetPx;

  return [posX, posY];
}
