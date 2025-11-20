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
import { useState } from 'react';

import { SearchMeasurementsForm } from '@/crystal_ball/components/search_measurements_form';
import {
  TimeSeriesChart,
  TimeSeriesLine,
} from '@/crystal_ball/components/time_series_chart';
import {
  useSearchMeasurements,
  SearchMeasurementsRequest,
  MeasurementRow,
} from '@/crystal_ball/hooks/use_android_perf_api';

interface Measurement {
  time: number;
  [valueKey: string]: number | undefined;
}

/**
 * Helper function to transform API response to chart data.
 * @param rows - from the API response.
 * @param metricKeys - from the SearchMeasurementsRequest.
 * @returns a list of measurements to be used by the Time Series chart.
 */
const transformDataForChart = (
  rows: MeasurementRow[],
  metricKeys: string[],
): Measurement[] => {
  const dataMap: { [time: number]: Measurement } = {};

  rows.forEach((row) => {
    if (
      !row.buildCreateTime ||
      row.metricKey === undefined ||
      row.value === undefined
    ) {
      return;
    }

    const time = new Date(row.buildCreateTime).getTime();

    if (!dataMap[time]) {
      dataMap[time] = { time };
      // Initialize all requested metric keys to undefined for this time slot
      metricKeys.forEach((key) => {
        dataMap[time][key] = undefined;
      });
    }

    // Set the value for the specific metricKey at this time
    if (metricKeys.includes(row.metricKey)) {
      dataMap[time][row.metricKey] = row.value / 1000;
    }
  });

  return Object.values(dataMap).sort((a, b) => a.time - b.time);
};

/**
 * A simple landing page component.
 */
export function LandingPage() {
  const [searchRequest, setSearchRequest] =
    useState<SearchMeasurementsRequest | null>(null);

  const {
    data: searchResponse,
    isLoading: isSearchLoading,
    isError: isSearchError,
    error: searchError,
  } = useSearchMeasurements(searchRequest!, {
    enabled: !!searchRequest,
  });

  const handleSearchSubmit = (request: SearchMeasurementsRequest) => {
    setSearchRequest(request);
  };

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
        initialRequest={searchRequest ?? {}}
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

      {!searchRequest && (
        <Typography variant="body1" sx={{ mt: 2 }}>
          Enter search parameters to view performance data.
        </Typography>
      )}

      {searchResponse && chartData.length === 0 && !isSearchLoading && (
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
