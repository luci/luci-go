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
import { useState, useEffect, useCallback, useMemo } from 'react';

import { TimeRangeSelector } from '@/common/components/time_range_selector';
import { getAbsoluteStartEndTime } from '@/common/components/time_range_selector/time_range_selector_utils';
import {
  SearchMeasurementsForm,
  TimeSeriesChart,
  TimeSeriesDataSet,
  WidgetContainer,
  BreakdownTableWidget,
} from '@/crystal_ball/components';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import { MOCK_BREAKDOWN_DATA } from '@/crystal_ball/components/mock_breakdown_data';
import { RequireLogin } from '@/crystal_ball/components/require_login';
import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';
import {
  useSearchMeasurements,
  useSearchQuerySync,
} from '@/crystal_ball/hooks';
import { validateSearchRequest } from '@/crystal_ball/utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  MeasurementRow,
  SearchMeasurementsRequest,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const GOLDEN_RATIO_CONJUGATE = 0.618033988749895;

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
const transformDataForChart = (
  rows: readonly MeasurementRow[],
  metricKeys: readonly string[],
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

/**
 * A simple demo page component.
 */
export function DemoPage() {
  const topBarAction = useMemo(() => <TimeRangeSelector />, []);
  useTopBarConfig('Demo Page', topBarAction);

  const { searchRequestFromUrl, updateSearchQuery } = useSearchQuerySync();
  const [searchParams] = useSyncedSearchParams();

  const timeOption = searchParams.get('time_option');
  const startTimeParam = searchParams.get('start_time');
  const endTimeParam = searchParams.get('end_time');

  const { startTime, endTime } = useMemo(() => {
    // The TimeRangeSelector syncs its state to the URL.
    // We manually extract those specific query parameters here to calculate the
    // absolute start and end times for the backend API request.
    const paramsForTime = new URLSearchParams();
    if (timeOption) paramsForTime.set('time_option', timeOption);
    if (startTimeParam) paramsForTime.set('start_time', startTimeParam);
    if (endTimeParam) paramsForTime.set('end_time', endTimeParam);

    return getAbsoluteStartEndTime(paramsForTime, DateTime.now());
  }, [timeOption, startTimeParam, endTimeParam]);

  const [searchRequest, setSearchRequest] = useState<SearchMeasurementsRequest>(
    {
      metricKeys: [],
      extraColumns: [],
    },
  );

  const [isInitialValid, setIsInitialValid] = useState(false);

  useEffect(() => {
    const hasInitialRequest = Object.keys(searchRequestFromUrl).length > 0;

    const newRequest: SearchMeasurementsRequest = {
      metricKeys: searchRequestFromUrl.metricKeys || [],
      extraColumns: searchRequestFromUrl.extraColumns || [],
      testNameFilter: searchRequestFromUrl.testNameFilter,
      atpTestNameFilter: searchRequestFromUrl.atpTestNameFilter,
      buildBranch: searchRequestFromUrl.buildBranch,
      buildTarget: searchRequestFromUrl.buildTarget,
      buildCreateStartTime: startTime?.toISO() || undefined,
      buildCreateEndTime: endTime?.toISO() || undefined,
    };

    setSearchRequest((prev) => {
      if (JSON.stringify(prev) === JSON.stringify(newRequest)) return prev;
      return newRequest;
    });

    if (hasInitialRequest) {
      const errors = validateSearchRequest(searchRequestFromUrl);
      setIsInitialValid(Object.keys(errors).length === 0);
    } else {
      setIsInitialValid(false);
    }
  }, [searchRequestFromUrl, startTime, endTime]);

  const {
    data: searchResponse,
    isLoading: isSearchLoading,
    isError: isSearchError,
    error: searchError,
  } = useSearchMeasurements(searchRequest, {
    enabled: !!searchRequest && isInitialValid,
  });

  const handleSearchSubmit = useCallback(
    (request: SearchMeasurementsRequest) => {
      setIsInitialValid(true);

      const fullRequest: SearchMeasurementsRequest = {
        metricKeys: request.metricKeys || [],
        extraColumns: request.extraColumns || [],
        testNameFilter: request.testNameFilter,
        atpTestNameFilter: request.atpTestNameFilter,
        buildBranch: request.buildBranch,
        buildTarget: request.buildTarget,
        pageToken: request.pageToken,
        pageSize: request.pageSize,
        buildCreateStartTime: startTime?.toISO() || undefined,
        buildCreateEndTime: endTime?.toISO() || undefined,
      };

      setSearchRequest(fullRequest);
      updateSearchQuery(request);
    },
    [updateSearchQuery, startTime, endTime],
  );

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

  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="h5" gutterBottom>
        Crystal Ball Performance Metrics
      </Typography>

      <SearchMeasurementsForm
        onSubmit={handleSearchSubmit}
        isSubmitting={isSearchLoading}
        initialRequest={searchRequestFromUrl}
      />

      {isSearchLoading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', my: 3 }}>
          <CircularProgress />
        </Box>
      )}

      {isSearchError && (
        <Alert severity="error" sx={{ my: 2 }}>
          {COMMON_MESSAGES.ERROR_FETCHING_MEASUREMENTS}
          {searchError?.message || COMMON_MESSAGES.UNKNOWN_ERROR}
        </Alert>
      )}

      {searchResponse && hasData && (
        <TimeSeriesChart
          series={chartSeries}
          chartTitle="Performance Metrics"
          yAxisLabel="Value"
        />
      )}

      {!isInitialValid && !isSearchLoading && (
        <Typography variant="body1" sx={{ mt: 2 }}>
          Enter search parameters to view performance data.
        </Typography>
      )}

      {searchRequest && !isSearchLoading && !isSearchError && !hasData && (
        <Typography variant="body1" sx={{ mt: 2 }}>
          {COMMON_MESSAGES.NO_DATA_FOUND}
        </Typography>
      )}

      {/* Temporary Mock Data Display */}
      <Box sx={{ mt: 4, minWidth: 0 }}>
        <WidgetContainer
          title="Breakdown Example (Mock Data)"
          disablePadding={true}
        >
          <BreakdownTableWidget data={MOCK_BREAKDOWN_DATA} />
        </WidgetContainer>
      </Box>
    </Box>
  );
}

/**
 * Component export for Demo Page.
 */
export function Component() {
  return (
    <RequireLogin>
      <DemoPage />
    </RequireLogin>
  );
}
