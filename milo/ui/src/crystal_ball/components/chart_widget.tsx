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
import { useMemo } from 'react';

import {
  ChartSeriesEditor,
  FilterEditor,
  TimeSeriesChart,
} from '@/crystal_ball/components';
import {
  ATP_TEST_NAME_COLUMN,
  COMMON_MESSAGES,
  GLOBAL_TIME_RANGE_COLUMN,
  GOLDEN_RATIO_CONJUGATE,
} from '@/crystal_ball/constants';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks';
import { isStringArray } from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfChartWidget,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
  PerfXAxisConfig,
  PerfXAxisConfig_Granularity,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const isDataPointsValid = (
  dataPoints: readonly { readonly [key: string]: unknown }[],
  xAxisDataKey: string,
  yAxisDataKey: string,
): dataPoints is { [axisDataKey: string]: number | string }[] => {
  if (dataPoints === null || dataPoints === undefined) return false;

  return (
    Array.isArray(dataPoints) &&
    dataPoints.every(
      (dataPoint) =>
        dataPoint !== null &&
        dataPoint !== undefined &&
        (typeof dataPoint[xAxisDataKey] === 'number' ||
          typeof dataPoint[xAxisDataKey] === 'string') &&
        (typeof dataPoint[yAxisDataKey] === 'number' ||
          typeof dataPoint[yAxisDataKey] === 'string'),
    )
  );
};

function dataPointsToData(
  dataPoints: { [axisDataKey: string]: number | string }[],
  xAxisDataKey: string,
  yAxisDataKey: string,
): Array<[number, number]> {
  return dataPoints.map((pt) => {
    const x =
      typeof pt[xAxisDataKey] === 'string'
        ? Date.parse(pt[xAxisDataKey])
        : pt[xAxisDataKey];
    const y =
      typeof pt[yAxisDataKey] === 'string'
        ? parseFloat(pt[yAxisDataKey])
        : pt[yAxisDataKey];
    return [x, y];
  });
}

interface ChartWidgetProps {
  onUpdate: (updatedWidget: PerfChartWidget) => void;
  widget: PerfChartWidget;
  dashboardName: string;
  widgetId: string;
  globalFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
  dataSpecs?: { [key: string]: PerfDataSpec };
}

export function ChartWidget({
  onUpdate,
  widget,
  widgetId,
  filterColumns,
  isLoadingFilterColumns,
  globalFilters,
  dataSpecs,
}: ChartWidgetProps) {
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

  const fetchRequest = useMemo(
    () => ({
      dashboardContent: PerfDashboardContent.fromPartial({
        globalFilters: globalFilters ?? [],
        dataSpecs: dataSpecs ?? {},
        widgets: [
          PerfWidget.fromPartial({
            id: widgetId,
            chart: PerfChartWidget.fromPartial({
              ...widget,
              xAxis:
                widget.xAxis ??
                PerfXAxisConfig.fromPartial({
                  column: GLOBAL_TIME_RANGE_COLUMN,
                  granularity: PerfXAxisConfig_Granularity.HOURLY,
                }),
            }),
          }),
        ],
      }),
      widgetId,
    }),
    [globalFilters, dataSpecs, widgetId, widget],
  );

  const hasAtpTestFilter = useMemo(() => {
    const allFilters = [...(globalFilters ?? []), ...(widget.filters ?? [])];
    return allFilters.some(
      (f) =>
        f.column === ATP_TEST_NAME_COLUMN &&
        f.textInput?.defaultValue?.values?.[0],
    );
  }, [globalFilters, widget.filters]);

  const {
    data: widgetResponse,
    isLoading: isWidgetLoading,
    isError: isWidgetError,
    error: widgetError,
  } = useFetchDashboardWidgetData(fetchRequest, {
    enabled: !!widgetId && hasAtpTestFilter,
  });

  const chartSeries = useMemo(() => {
    if (!widgetResponse?.multiMetricChartData?.lines) return [];

    const xAxisKey = widgetResponse.multiMetricChartData.xAxisDataKey;
    const yAxisKey = widgetResponse.multiMetricChartData.yAxisDataKey;

    return widgetResponse.multiMetricChartData.lines.map((line, index) => {
      const seriesConfig = widget.series?.find(
        (s) => s.displayName === line.legendLabel,
      );
      return {
        name: line.legendLabel,
        data: isDataPointsValid(line.dataPoints, xAxisKey, yAxisKey)
          ? dataPointsToData(line.dataPoints, xAxisKey, yAxisKey)
          : [],
        stroke:
          seriesConfig?.color ??
          `hsl(${((index * GOLDEN_RATIO_CONJUGATE) % 1) * 360}, 70%, 50%)`,
      };
    });
  }, [widgetResponse, widget.series]);

  const hasData = useMemo(
    () => chartSeries.some((series) => series.data.length > 0),
    [chartSeries],
  );

  const widgetFilterColumns = useMemo(
    () =>
      filterColumns.filter(
        (c) =>
          c.applicableScopes?.includes(
            MeasurementFilterColumn_FilterScope.WIDGET,
          ) ||
          (isStringArray(c.applicableScopes) &&
            c.applicableScopes.includes('WIDGET')),
      ),
    [filterColumns],
  );

  return (
    <Box>
      <FilterEditor
        title="Widget Filters"
        filters={[...(widget.filters || [])]}
        onUpdateFilters={handleFiltersUpdate}
        dataSpecId={widget.dataSpecId}
        availableColumns={widgetFilterColumns}
        isLoadingColumns={isLoadingFilterColumns}
      />
      <Box sx={{ position: 'relative', minHeight: '300px', mt: 2 }}>
        {isWidgetLoading && (
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
        {isWidgetError && (
          <Alert severity="error" sx={{ my: 2 }}>
            {COMMON_MESSAGES.ERROR_FETCHING_MEASUREMENTS}
            {widgetError?.message || COMMON_MESSAGES.UNKNOWN_ERROR}
          </Alert>
        )}
        {!isWidgetLoading &&
          !isWidgetError &&
          (!widgetResponse || !hasData) && (
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
                {!hasAtpTestFilter
                  ? COMMON_MESSAGES.ATP_TEST_NAME_REQUIRED
                  : COMMON_MESSAGES.NO_DATA_FOUND}
              </Typography>
            </Box>
          )}
        {!isWidgetError && widgetResponse && hasData && (
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
        globalFilters={globalFilters}
        widgetFilters={widget.filters}
      />
    </Box>
  );
}
