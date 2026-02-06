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

import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import { useState, useEffect, useCallback, useMemo } from 'react';

import {
  EditableMarkdown,
  SearchMeasurementsForm,
  TimeSeriesChart,
  TimeSeriesDataSet,
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

/**
 * A simple demo page component.
 */
export function DemoPage() {
  const { searchRequestFromUrl, updateSearchQuery } = useSearchQuerySync();

  const [searchRequest, setSearchRequest] =
    useState<SearchMeasurementsRequest | null>(null);

  const [isInitialValid, setIsInitialValid] = useState(false);
  const [aboutMarkdown, setAboutMarkdown] = useState(
    '**Welcome to Crystal Ball!**\n\nThis is a placeholder.',
  );

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

  const chartSeries = useMemo(() => {
    const requestedMetricKeys = searchRequest?.metricKeys || [];
    return searchResponse?.rows
      ? transformDataForChart(searchResponse.rows, requestedMetricKeys)
      : [];
  }, [searchResponse, searchRequest]);

  return (
    <Box sx={{ padding: 2 }}>
      <Typography variant="h5" gutterBottom>
        Crystal Ball Performance Metrics
      </Typography>

      <EditableMarkdown
        initialMarkdown={aboutMarkdown}
        onSave={setAboutMarkdown}
      />

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

      {searchResponse && chartSeries.length > 0 && (
        <TimeSeriesChart
          series={chartSeries}
          chartTitle="Performance Metrics"
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
        chartSeries.length === 0 && (
          <Typography variant="body1" sx={{ mt: 2 }}>
            No data found for the given parameters.
          </Typography>
        )}
    </Box>
  );
}

/**
 * Component export for Demo Page.
 */
export function Component() {
  return <DemoPage />;
}
