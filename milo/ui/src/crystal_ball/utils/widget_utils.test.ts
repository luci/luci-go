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

import {
  PerfChartSeries_PerfAggregationFunction,
  PerfChartWidget,
  PerfChartWidget_ChartType,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { sanitizeChartWidget } from './widget_utils';

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
