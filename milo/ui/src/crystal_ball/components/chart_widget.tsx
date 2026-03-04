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

import { Alert, Box, CircularProgress, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { getAbsoluteStartEndTime } from '@/common/components/time_range_selector/time_range_selector_utils';
import { TimeSeriesChart, TimeSeriesDataSet } from '@/crystal_ball/components';
import { GOLDEN_RATIO_CONJUGATE } from '@/crystal_ball/constants';
import { useSearchMeasurements } from '@/crystal_ball/hooks';
import {
  MeasurementRow,
  PerfChartWidget,
  SearchMeasurementsRequest,
} from '@/crystal_ball/types';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

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

interface ChartWidgetProps {
  widget: PerfChartWidget;
}

export function ChartWidget({ widget }: ChartWidgetProps) {
  const [searchParams] = useSyncedSearchParams();

  // TODO: b/475638132 - Read time filters from PerfChartWidget
  const timeOption = searchParams.get('time_option');
  const startTimeParam = searchParams.get('start_time');
  const endTimeParam = searchParams.get('end_time');

  const { startTime, endTime } = useMemo(() => {
    const paramsForTime = new URLSearchParams();
    if (timeOption) paramsForTime.set('time_option', timeOption);
    if (startTimeParam) paramsForTime.set('start_time', startTimeParam);
    if (endTimeParam) paramsForTime.set('end_time', endTimeParam);

    return getAbsoluteStartEndTime(paramsForTime, DateTime.now());
  }, [timeOption, startTimeParam, endTimeParam]);

  const searchRequest: SearchMeasurementsRequest = useMemo(() => {
    const metricKeys =
      widget.series?.map((s) => s.metricField).filter(Boolean) || [];
    const request: SearchMeasurementsRequest = {
      metricKeys,
      buildCreateStartTime: startTime
        ? { seconds: startTime.toUnixInteger().toString(), nanos: 0 }
        : undefined,
      buildCreateEndTime: endTime
        ? { seconds: endTime.toUnixInteger().toString(), nanos: 0 }
        : undefined,
    };

    // TODO: b/475638132 - Build StreamMeasurementsRequest instead
    // Apply filters from the widget configuration
    widget.filters?.forEach((filter) => {
      if (!filter.column || !filter.textInput?.defaultValue?.values?.length)
        return;

      const value = filter.textInput?.defaultValue?.values[0];

      if (!value) return;

      let operator = 'EQUAL';
      let filterValue = value;

      if (filter.textInput?.defaultValue?.filterOperator) {
        operator = filter.textInput.defaultValue.filterOperator;
      }

      // Adjust value for LIKE operators
      if (operator === 'STARTS_WITH') {
        filterValue = value + '%';
      } else if (operator === 'CONTAINS') {
        filterValue = '%' + value + '%';
      } else if (operator === 'ENDS_WITH') {
        filterValue = '%' + value;
      } else if (operator === 'LIKE') {
        filterValue = value; // Assume user provided wildcards
      }

      switch (filter.column) {
        case 'test_name':
          request.testNameFilter = filterValue;
          break;
        case 'atp_test_name':
          request.atpTestNameFilter = filterValue;
          break;
        case 'build_branch':
          request.buildBranch = value;
          break;
        case 'build_target':
          request.buildTarget = value;
          break;
      }
    });

    return request;
  }, [widget, startTime, endTime]);

  const {
    data: searchResponse,
    isLoading: isSearchLoading,
    isError: isSearchError,
    error: searchError,
  } = useSearchMeasurements(searchRequest, {
    enabled: !!searchRequest.metricKeys?.length,
  });

  const chartSeries = useMemo(() => {
    const requestedMetricKeys = searchRequest?.metricKeys || [];
    return searchResponse?.rows
      ? transformDataForChart(searchResponse.rows, requestedMetricKeys)
      : [];
  }, [searchResponse, searchRequest]);

  const hasData = useMemo(
    () => chartSeries.some((series) => series.data.length > 0),
    [chartSeries],
  );

  if (isSearchLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', my: 3 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (isSearchError) {
    return (
      <Alert severity="error" sx={{ my: 2 }}>
        Error fetching chart data: {searchError?.message || 'Unknown error'}
      </Alert>
    );
  }

  if (!searchResponse || !hasData) {
    return (
      <Typography variant="body1" sx={{ mt: 2, p: 2 }}>
        No data found for the given parameters.
      </Typography>
    );
  }

  return (
    <TimeSeriesChart
      series={chartSeries}
      chartTitle={widget.displayName || 'Performance Metrics'}
      yAxisLabel="Value"
    />
  );
}
