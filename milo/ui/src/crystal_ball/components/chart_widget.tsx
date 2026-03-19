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

import {
  ChartSeriesEditor,
  FilterEditor,
  TimeSeriesChart,
} from '@/crystal_ball/components';
import { GLOBAL_TIME_RANGE_FILTER_ID } from '@/crystal_ball/constants/api';
import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';
import { useSearchMeasurements } from '@/crystal_ball/hooks';
import { transformDataForChart } from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  PerfChartSeries,
  PerfChartWidget,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  SearchMeasurementsRequest,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface ChartWidgetProps {
  onUpdate: (updatedWidget: PerfChartWidget) => void;
  widget: PerfChartWidget;
  globalFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
}

export function ChartWidget({
  onUpdate,
  widget,
  globalFilters,
  filterColumns,
  isLoadingFilterColumns,
}: ChartWidgetProps) {
  const { startTime, endTime, lastNDays } = useMemo(() => {
    const timeFilter = globalFilters?.find(
      (f) => f.id === GLOBAL_TIME_RANGE_FILTER_ID,
    );

    if (!timeFilter?.range?.defaultValue) {
      return { startTime: null, endTime: null, lastNDays: 3 };
    }

    const operator = perfFilterDefault_FilterOperatorFromJSON(
      timeFilter.range.defaultValue.filterOperator,
    );
    const values = timeFilter.range.defaultValue.values;

    if (operator === PerfFilterDefault_FilterOperator.IN_PAST) {
      const option = values[0];
      if (option) {
        const match = option.match(/^(\d+)([mhdw])$/i);
        if (match) {
          const num = parseInt(match[1], 10);
          const unit = match[2].toLowerCase();

          if (unit === 'd') {
            return { startTime: null, endTime: null, lastNDays: num };
          } else if (unit === 'w') {
            return { startTime: null, endTime: null, lastNDays: num * 7 };
          } else {
            const durationObj: Record<string, number> = {};
            if (unit === 'h') durationObj.hours = num;
            else if (unit === 'm') durationObj.minutes = num;

            return {
              startTime: DateTime.utc().minus(durationObj),
              endTime: null,
              lastNDays: undefined,
            };
          }
        }
      }
      return { startTime: null, endTime: null, lastNDays: undefined };
    }

    if (!values) {
      return { startTime: null, endTime: null, lastNDays: undefined };
    }

    return {
      startTime:
        values[0] && values[0] !== ''
          ? DateTime.fromISO(values[0]).toUTC()
          : null,
      endTime:
        values[1] && values[1] !== ''
          ? DateTime.fromISO(values[1]).toUTC()
          : null,
      lastNDays: undefined,
    };
  }, [globalFilters]);

  const searchRequest: SearchMeasurementsRequest = useMemo(() => {
    const metricKeys =
      widget.series?.map((s) => s.metricField).filter(Boolean) || [];

    let testNameFilter: string | undefined;
    let atpTestNameFilter: string | undefined;
    let buildBranch: string | undefined;
    let buildTarget: string | undefined;

    // TODO: b/475638132 - Build StreamMeasurementsRequest instead
    // Apply filters from the widget configuration
    widget.filters?.forEach((filter) => {
      if (!filter.column || !filter.textInput?.defaultValue?.values?.length)
        return;

      const value = filter.textInput?.defaultValue?.values[0];

      if (!value) return;

      let operator = PerfFilterDefault_FilterOperator.EQUAL;
      let filterValue = value;

      if (filter.textInput?.defaultValue?.filterOperator !== undefined) {
        operator = perfFilterDefault_FilterOperatorFromJSON(
          filter.textInput.defaultValue.filterOperator,
        );
      }

      // Adjust value for LIKE operators
      if (operator === PerfFilterDefault_FilterOperator.STARTS_WITH) {
        filterValue = value + '%';
      } else if (operator === PerfFilterDefault_FilterOperator.CONTAINS) {
        filterValue = '%' + value + '%';
      } else if (operator === PerfFilterDefault_FilterOperator.ENDS_WITH) {
        filterValue = '%' + value;
      } else if (operator === PerfFilterDefault_FilterOperator.LIKE) {
        filterValue = value; // Assume user provided wildcards
      }

      switch (filter.column) {
        case 'test_name':
          testNameFilter = filterValue;
          break;
        case 'atp_test_name':
          atpTestNameFilter = filterValue;
          break;
        case 'build_branch':
          buildBranch = value;
          break;
        case 'build_target':
          buildTarget = value;
          break;
      }
    });

    const request: SearchMeasurementsRequest = {
      testNameFilter,
      atpTestNameFilter,
      buildBranch,
      buildTarget,
      metricKeys,
      extraColumns: [],
      buildCreateStartTime: startTime?.toISO() || undefined,
      buildCreateEndTime: endTime?.toISO() || undefined,
      lastNDays,
    };

    return request;
  }, [widget, startTime, endTime, lastNDays]);

  const handleFiltersUpdate = (updatedFilters: PerfFilter[]) => {
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        filters: updatedFilters,
      }),
    );
  };

  const handleSeriesUpdate = (updatedSeries: PerfChartSeries[]) => {
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        series: updatedSeries,
      }),
    );
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
            {COMMON_MESSAGES.ERROR_FETCHING_MEASUREMENTS}
            {searchError?.message || COMMON_MESSAGES.UNKNOWN_ERROR}
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
                {COMMON_MESSAGES.NO_DATA_FOUND}
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
        series={[...(widget.series || [])]}
        onUpdateSeries={handleSeriesUpdate}
        dataSpecId={widget.dataSpecId}
      />
      <FilterEditor
        filters={[...(widget.filters || [])]}
        onUpdateFilters={handleFiltersUpdate}
        dataSpecId={widget.dataSpecId}
        availableColumns={filterColumns}
        isLoadingColumns={isLoadingFilterColumns}
      />
    </Box>
  );
}
