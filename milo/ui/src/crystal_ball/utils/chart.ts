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

import { TimeSeriesDataSet } from '@/crystal_ball/components';
import { GOLDEN_RATIO_CONJUGATE } from '@/crystal_ball/constants';
import { MeasurementRow } from '@/crystal_ball/types';

/**
 * On a per timestamp and metric key basis, total values based on build ids.
 */
interface AggregationData {
  sum: number;
  count: number;
}

/**
 * Helper function to transform API response to chart data,
 * aggregating by mean across buildId.
 * @param rows - from the API response.
 * @param metricKeys - from the SearchMeasurementsRequest.
 * @returns a list of time series datasets.
 */
export const transformDataForChart = (
  rows: MeasurementRow[],
  metricKeys: string[],
): TimeSeriesDataSet[] => {
  const dataMap: {
    [time: number]: { [metricKey: string]: AggregationData };
  } = {};

  rows.forEach((row) => {
    if (
      !row.buildCreateTime ||
      row.metricKey === undefined ||
      row.value === undefined ||
      row.buildId === undefined
    ) {
      return;
    }

    // Skip rows that don't match the requested metric keys
    if (!metricKeys.includes(row.metricKey)) {
      return;
    }

    const time = new Date(row.buildCreateTime).getTime();

    if (!dataMap[time]) {
      dataMap[time] = {};
    }

    if (!dataMap[time][row.metricKey]) {
      dataMap[time][row.metricKey] = { sum: 0, count: 0 };
    }

    dataMap[time][row.metricKey].sum += row.value;
    dataMap[time][row.metricKey].count += 1;
  });

  const sortedTimes = Object.keys(dataMap)
    .map(Number)
    .sort((a, b) => a - b);

  return metricKeys.map((key, index) => {
    const data: [number, number][] = [];
    sortedTimes.forEach((time) => {
      // Calculate the mean for the metricKey at this time
      const agg = dataMap[time][key];
      if (agg) {
        data.push([time, agg.sum / agg.count]);
      }
    });

    return {
      name: key,
      data,
      // Use golden ratio to generate distinct colors
      stroke: `hsl(${((index * GOLDEN_RATIO_CONJUGATE) % 1) * 360}, 70%, 50%)`,
    };
  });
};
