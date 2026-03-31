// Copyright 2025 The LUCI Authors.
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

import { Column, GLOBAL_TIME_RANGE_COLUMN } from '@/crystal_ball/constants';
import {
  PerfChartSeries_PerfAggregationFunction,
  PerfXAxisConfig,
  PerfXAxisConfig_Granularity,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

export const GROUP_BY_CONFIG: Record<
  string,
  { label: string; xAxis: PerfXAxisConfig }
> = {
  BUILD_ID: {
    label: 'Build ID',
    xAxis: PerfXAxisConfig.fromPartial({
      column: Column.BUILD_ID,
      granularity: PerfXAxisConfig_Granularity.PER_BUILD,
    }),
  },
  TIMESTAMP: {
    label: 'Timestamp',
    xAxis: PerfXAxisConfig.fromPartial({
      column: GLOBAL_TIME_RANGE_COLUMN,
      granularity: PerfXAxisConfig_Granularity.PER_VALUE,
    }),
  },
  HOUR: {
    label: 'Hour',
    xAxis: PerfXAxisConfig.fromPartial({
      column: GLOBAL_TIME_RANGE_COLUMN,
      granularity: PerfXAxisConfig_Granularity.HOURLY,
    }),
  },
  DAY: {
    label: 'Day',
    xAxis: PerfXAxisConfig.fromPartial({
      column: GLOBAL_TIME_RANGE_COLUMN,
      granularity: PerfXAxisConfig_Granularity.DAILY,
    }),
  },
  WEEK: {
    label: 'Week',
    xAxis: PerfXAxisConfig.fromPartial({
      column: GLOBAL_TIME_RANGE_COLUMN,
      granularity: PerfXAxisConfig_Granularity.WEEKLY,
    }),
  },
};

export const GROUP_BY_OPTIONS = Object.entries(GROUP_BY_CONFIG).map(
  ([value, { label }]) => ({
    value,
    label,
  }),
);

export const DEFAULT_GROUP_BY = 'TIMESTAMP';

export const getGroupByFromGranularity = (
  granularity: PerfXAxisConfig_Granularity,
): string => {
  const entry = Object.entries(GROUP_BY_CONFIG).find(
    ([_, config]) => config.xAxis.granularity === granularity,
  );
  return entry ? entry[0] : DEFAULT_GROUP_BY;
};

export const DEFAULT_X_AXIS_CONFIG = PerfXAxisConfig.fromPartial({
  column: GLOBAL_TIME_RANGE_COLUMN,
  granularity: PerfXAxisConfig_Granularity.HOURLY,
});

export const AGGREGATION_FUNCTION_LABELS: Record<
  PerfChartSeries_PerfAggregationFunction,
  string
> = {
  [PerfChartSeries_PerfAggregationFunction.PERF_AGGREGATION_FUNCTION_UNSPECIFIED]:
    'Default',
  [PerfChartSeries_PerfAggregationFunction.MEAN]: 'Mean',
  [PerfChartSeries_PerfAggregationFunction.P50]: 'P50 (Median)',
  [PerfChartSeries_PerfAggregationFunction.P75]: 'P75',
  [PerfChartSeries_PerfAggregationFunction.P90]: 'P90',
  [PerfChartSeries_PerfAggregationFunction.P99]: 'P99',
  [PerfChartSeries_PerfAggregationFunction.MIN]: 'Min',
  [PerfChartSeries_PerfAggregationFunction.MAX]: 'Max',
  [PerfChartSeries_PerfAggregationFunction.COUNT]: 'Count',
};
