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

import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import { useState, useEffect, useCallback } from 'react';

import {
  SearchMeasurementsForm,
  TimeSeriesChart,
  TimeSeriesLine,
} from '@/crystal_ball/components';
import {
  useSearchMeasurements,
  useSearchQuerySync,
} from '@/crystal_ball/hooks';
import {
  MeasurementRow,
  SearchMeasurementsRequest,
} from '@/crystal_ball/types';
import { validateSearchRequest } from '@/crystal_ball/utils';

/**
 * Represents a single build create time to various metric key value mappings.
 */
interface Measurement {
  time: number;
  [valueKey: string]: number | undefined;
}

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
 * @returns a list of measurements to be used by the Time Series chart.
 */
const transformDataForChart = (
  rows: MeasurementRow[],
  metricKeys: string[],
): Measurement[] => {
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

  const chartData: Measurement[] = Object.keys(dataMap)
    .map(Number)
    .sort((a, b) => a - b)
    .map((time) => {
      const measurement: Measurement = { time };

      // Initialize all requested metric keys to undefined for this time slot
      metricKeys.forEach((key) => {
        measurement[key] = undefined;
      });

      // Calculate the mean for each metricKey at this time
      metricKeys.forEach((key) => {
        if (dataMap[time][key]) {
          const agg = dataMap[time][key];
          measurement[key] = agg.sum / agg.count;
        }
      });

      return measurement;
    });

  return chartData;
};

/**
 * A simple landing page component.
 */
export function LandingPage() {
  const { searchRequestFromUrl, updateSearchQuery } = useSearchQuerySync();

  const [searchRequest, setSearchRequest] =
    useState<SearchMeasurementsRequest | null>(null);

  const [isInitialValid, setIsInitialValid] = useState(false);

  useEffect(() => {
    const hasInitialRequest = Object.keys(searchRequestFromUrl).length > 0;
    if (hasInitialRequest) {
      const errors = validateSearchRequest(searchRequestFromUrl);
      if (Object.keys(errors).length === 0) {
        setSearchRequest(searchRequestFromUrl as SearchMeasurementsRequest);
        setIsInitialValid(true);
      } else {
        setSearchRequest(null);
        setIsInitialValid(false);
      }
    } else {
      setSearchRequest(null);
      setIsInitialValid(false);
    }
  }, [searchRequestFromUrl]);

  const {
    data: searchResponse,
    isLoading: isSearchLoading,
    isError: isSearchError,
    error: searchError,
  } = useSearchMeasurements(searchRequest!, {
    enabled: !!searchRequest && isInitialValid,
  });

  const handleSearchSubmit = useCallback(
    (request: SearchMeasurementsRequest) => {
      setIsInitialValid(true);
      setSearchRequest(request);
      updateSearchQuery(request);
    },
    [updateSearchQuery],
  );

  const requestedMetricKeys = searchRequest?.metricKeys || [];

  const chartData = searchResponse?.rows
    ? transformDataForChart(searchResponse.rows, requestedMetricKeys)
    : [];

  const chartLines: TimeSeriesLine[] = requestedMetricKeys.map(
    (key, index) => ({
      dataKey: key,
      // Cycle through some colors
      stroke: ['#1976d2', '#d21976', '#76d219', '#d27619'][index % 4],
      name: key,
    }),
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
          Error fetching measurements: {searchError?.message || 'Unknown error'}
        </Alert>
      )}

      {searchResponse && chartData.length > 0 && (
        <TimeSeriesChart
          data={chartData}
          lines={chartLines}
          chartTitle="Performance Metrics"
          xAxisDataKey="time"
          yAxisLabel="Value"
        />
      )}

      {!searchRequest && !isSearchLoading && (
        <Typography variant="body1" sx={{ mt: 2 }}>
          Enter search parameters to view performance data.
        </Typography>
      )}

      {searchRequest &&
        !isSearchLoading &&
        !isSearchError &&
        chartData.length === 0 && (
          <Typography variant="body1" sx={{ mt: 2 }}>
            No data found for the given parameters.
          </Typography>
        )}
    </Box>
  );
}

/**
 * Component export for Landing Page.
 */
export function Component() {
  return <LandingPage />;
}
