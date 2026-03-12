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
import {
  ChartSeriesEditor,
  FilterEditor,
  TimeSeriesChart,
} from '@/crystal_ball/components';
import { useSearchMeasurements } from '@/crystal_ball/hooks';
import {
  PerfChartSeries,
  PerfChartWidget,
  PerfFilter,
  SearchMeasurementsRequest,
} from '@/crystal_ball/types';
import { transformDataForChart } from '@/crystal_ball/utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

interface ChartWidgetProps {
  onUpdate: (updatedWidget: PerfChartWidget) => void;
  widget: PerfChartWidget;
  filterColumns: string[];
}

export function ChartWidget({
  onUpdate,
  widget,
  filterColumns,
}: ChartWidgetProps) {
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

  const handleFiltersUpdate = (updatedFilters: PerfFilter[]) => {
    onUpdate({
      ...widget,
      filters: updatedFilters,
    });
  };

  const handleSeriesUpdate = (updatedSeries: PerfChartSeries[]) => {
    onUpdate({
      ...widget,
      series: updatedSeries,
    });
  };

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

  return (
    <Box>
      <Box sx={{ position: 'relative', minHeight: '300px' }}>
        {isSearchLoading && (
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              backgroundColor: 'rgba(255, 255, 255, 0.7)',
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              zIndex: 10,
              borderRadius: 1,
            }}
          >
            <CircularProgress />
          </Box>
        )}
        {isSearchError && (
          <Alert severity="error" sx={{ my: 2 }}>
            Error fetching chart data: {searchError?.message || 'Unknown error'}
          </Alert>
        )}
        {!isSearchLoading &&
          !isSearchError &&
          (!searchResponse || !hasData) && (
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
                minHeight: '300px',
              }}
            >
              <Typography
                variant="body1"
                sx={{ p: 2, color: 'text.secondary' }}
              >
                No data found for the given parameters.
              </Typography>
            </Box>
          )}
        {!isSearchError && searchResponse && hasData && (
          <TimeSeriesChart
            series={chartSeries}
            chartTitle={widget.displayName || 'Performance Metrics'}
            yAxisLabel="Value"
          />
        )}
      </Box>
      <ChartSeriesEditor
        series={widget.series || []}
        onUpdateSeries={handleSeriesUpdate}
        dataSpecId={widget.dataSpecId}
      />
      <FilterEditor
        filters={widget.filters || []}
        onUpdateFilters={handleFiltersUpdate}
        dataSpecId={widget.dataSpecId}
        availableColumns={filterColumns}
      />
    </Box>
  );
}
