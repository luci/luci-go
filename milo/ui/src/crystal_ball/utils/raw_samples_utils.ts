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
  FetchWidgetRawSamplesRequest,
  PerfChartWidget,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ChartPoint } from './widget_utils';

/**
 * Converts a chart data point into the selection context used to fetch the
 * raw samples backing that point. The aggregated value and row count are
 * excluded since they are not dimensions of the point.
 */
export function pointToSelectionContext(
  point: ChartPoint | undefined,
): Record<string, string> {
  const selectionContext: Record<string, string> = {};
  if (point) {
    Object.entries(point).forEach(([key, val]) => {
      if (key !== 'value' && key !== NUM_AGGREGATED_ROWS) {
        selectionContext[key] = String(val);
      }
    });
  }
  return selectionContext;
}

/**
 * Parameters for creating a FetchWidgetRawSamplesRequest.
 */
export interface RawSamplesRequestParams {
  dashboardName: string;
  dataSpecs?: { [key: string]: PerfDataSpec };
  extraColumns: readonly string[];
  globalFilters?: readonly PerfFilter[];
  orderBy: string;
  pageSize: number;
  pageToken?: string;
  selectionContext: Record<string, string>;
  seriesId?: string;
  seriesIndex?: number;
  widget: PerfChartWidget;
  widgetId: string;
}

/**
 * Creates a request to fetch the raw samples for a widget series, narrowed
 * down by the given selection context.
 */
export function createRawSamplesRequest({
  dashboardName,
  dataSpecs,
  extraColumns,
  globalFilters,
  orderBy,
  pageSize,
  pageToken,
  selectionContext,
  seriesId,
  seriesIndex,
  widget,
  widgetId,
}: RawSamplesRequestParams): FetchWidgetRawSamplesRequest {
  return FetchWidgetRawSamplesRequest.fromPartial({
    name: dashboardName,
    dashboardContent: PerfDashboardContent.fromPartial({
      globalFilters: globalFilters ?? [],
      dataSpecs: dataSpecs ?? {},
      widgets: [
        PerfWidget.fromPartial({
          id: widgetId,
          chart: PerfChartWidget.fromPartial({
            ...widget,
          }),
        }),
      ],
    }),
    widgetId,
    seriesId,
    seriesIndex:
      seriesIndex !== undefined && seriesIndex >= 0 ? seriesIndex : undefined,
    selectionContext,
    pageSize,
    orderBy,
    pageToken,
    extraColumns: [...extraColumns],
  });
}
