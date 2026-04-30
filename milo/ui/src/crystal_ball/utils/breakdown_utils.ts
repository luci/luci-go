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

import { OPERATOR_DISPLAY_NAMES } from '@/crystal_ball/constants';
import { parseSingleFilter } from '@/crystal_ball/utils';
import {
  BreakdownSection,
  BreakdownTableConfig_BreakdownAggregation,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Represents a single row of breakdown data.
 */
export type BreakdownRow = Record<string, string | number | null | undefined>;

/**
 * The key used for the dimension column in the breakdown table.
 */
export const DIMENSION_COLUMN_KEY = 'dimension_value';

/**
 * The fallback label used for unknown dimensions.
 */
export const UNKNOWN_DIMENSION = 'UNKNOWN';

const BREAKDOWN_METRIC_KEYS = new Set(
  Object.keys(BreakdownTableConfig_BreakdownAggregation).filter(
    (key) => isNaN(Number(key)) && key !== 'BREAKDOWN_AGGREGATION_UNSPECIFIED',
  ),
);

/**
 * Derives the dimension key and metric columns from breakdown data.
 */
export function deriveTableColumns(
  data: readonly Readonly<Record<string, string | number | null | undefined>>[],
) {
  if (!data.length)
    return { dimensionKey: DIMENSION_COLUMN_KEY, metricColumns: [] };

  const keys = Object.keys(data[0]);
  const dimensionKey =
    keys.find((key) => {
      const isMetric = Array.from(BREAKDOWN_METRIC_KEYS).some(
        (k) => k.toUpperCase() === key.toUpperCase(),
      );
      return !isMetric;
    }) ?? DIMENSION_COLUMN_KEY;

  const metricColumns = keys.filter((key) => key !== dimensionKey);

  return { dimensionKey, metricColumns };
}

export function formatFilters(filters?: readonly PerfFilter[]) {
  return (
    filters?.flatMap((f) => {
      const parsed = parseSingleFilter(f);
      return parsed.map((p) => {
        const operatorStr =
          OPERATOR_DISPLAY_NAMES[p.operator] ??
          PerfFilterDefault_FilterOperator[p.operator] ??
          '=';

        const safeOperatorStr = operatorStr.startsWith('=')
          ? `'${operatorStr}`
          : operatorStr;

        return ['', p.column, safeOperatorStr, String(p.value)];
      });
    }) ?? []
  );
}

/**
 * Prepares data for export to sheets.
 */
export function prepareExportData(
  activeSection: BreakdownSection,
  metricField?: string,
  globalFilters?: readonly PerfFilter[],
  widgetFilters?: readonly PerfFilter[],
  seriesFilters?: readonly PerfFilter[],
) {
  const data = activeSection.rows ?? [];
  const { dimensionKey, metricColumns } = deriveTableColumns(data);

  const headers = [
    (activeSection.dimensionColumn ?? UNKNOWN_DIMENSION)
      .replace(/_/g, ' ')
      .toUpperCase(),
    ...metricColumns.map((c) => c.toUpperCase()),
  ];

  const values = data.map((row) => [
    row[dimensionKey],
    ...metricColumns.map((c) => row[c]),
  ]);

  const metadataRows: (string | number | null | undefined)[][] = [
    ['Metric', metricField ?? 'N/A'],
    ['Generated at', new Date().toISOString()],
    ['Filters'],
  ];

  if (globalFilters?.length) {
    metadataRows.push(['  Global'], ...formatFilters(globalFilters));
  }
  if (widgetFilters?.length) {
    metadataRows.push(['  Widget'], ...formatFilters(widgetFilters));
  }
  if (seriesFilters?.length) {
    metadataRows.push(['  Series'], ...formatFilters(seriesFilters));
  }

  metadataRows.push([]); // Empty row separator

  return {
    title: `Breakdown by ${activeSection.dimensionColumn}`,
    values: [...metadataRows, headers, ...values],
  };
}
