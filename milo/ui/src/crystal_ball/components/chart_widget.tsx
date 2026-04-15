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
import {
  Alert,
  Box,
  CircularProgress,
  Divider,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';

import {
  ChartSeriesEditor,
  ChartTooltipParam,
  ChartWidgetToolbar,
  FilterEditor,
  TimeSeriesChart,
} from '@/crystal_ball/components';
import {
  Column,
  COMMON_MESSAGES,
  DEFAULT_X_AXIS_CONFIG,
  getGroupByFromGranularity,
  GROUP_BY_CONFIG,
  NUM_AGGREGATED_ROWS,
} from '@/crystal_ball/constants';
import {
  useFetchDashboardWidgetData,
  EditorUiKeyPrefix,
} from '@/crystal_ball/hooks';
import {
  dataPointsToData,
  generateColor,
  getSafeChartType,
  isDataPointsValid,
  isStringArray,
  parseSingleFilter,
} from '@/crystal_ball/utils';
import {
  FetchDashboardWidgetDataResponse,
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfChartSeries_PerfAggregationFunction,
  perfChartSeries_PerfAggregationFunctionFromJSON,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
  PerfWidget,
  perfXAxisConfig_GranularityFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Helper to get chart series based on chart type.
 */
function getChartSeries(
  chartType: PerfChartWidget_ChartType,
  widgetResponse: FetchDashboardWidgetDataResponse | undefined,
  widgetSeries: readonly PerfChartSeries[] | undefined,
  hiddenSeriesNames: Set<string>,
) {
  switch (chartType) {
    case PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION: {
      if (!widgetResponse?.invocationDistributionData?.points) return [];
      const xAxisKey = widgetResponse.invocationDistributionData.xAxisDataKey;
      const yAxisKey = widgetResponse.invocationDistributionData.yAxisDataKey;

      return widgetResponse.invocationDistributionData.points
        .map((group, index) => {
          const seriesConfig = widgetSeries?.find(
            (s) => s.displayName === group.legendLabel,
          );
          return {
            name: group.legendLabel,
            data: (group.points ?? []).map(
              (point): { x: number; y: number; count: number } => {
                const xValue = point[xAxisKey];
                let x: number;
                if (typeof xValue === 'number') {
                  x = xValue;
                } else if (typeof xValue === 'string') {
                  if (!isNaN(Number(xValue)) && isFinite(Number(xValue))) {
                    x = Number(xValue);
                  } else {
                    const parsedDate = Date.parse(xValue);
                    x = isNaN(parsedDate) ? 0 : parsedDate;
                  }
                } else {
                  x = 0;
                }

                const yValue = point[yAxisKey];
                const y =
                  typeof yValue === 'string'
                    ? parseFloat(yValue)
                    : typeof yValue === 'number'
                      ? yValue
                      : 0;
                const countVal = point[NUM_AGGREGATED_ROWS];
                const count = typeof countVal === 'number' ? countVal : 1;
                return { x, y, count };
              },
            ),
            stroke: seriesConfig?.color ?? generateColor(index),
          };
        })
        .filter((series) => !hiddenSeriesNames.has(series.name));
    }
    default: {
      if (!widgetResponse?.multiMetricChartData?.lines) return [];

      const xAxisKey = widgetResponse.multiMetricChartData.xAxisDataKey;
      const yAxisKey = widgetResponse.multiMetricChartData.yAxisDataKey;

      return widgetResponse.multiMetricChartData.lines
        .map((line, index) => {
          const seriesConfig = widgetSeries?.find(
            (s) => s.displayName === line.legendLabel,
          );
          return {
            name: line.legendLabel,
            data: isDataPointsValid(line.dataPoints, xAxisKey, yAxisKey)
              ? dataPointsToData(line.dataPoints, xAxisKey, yAxisKey)
              : [],
            stroke: seriesConfig?.color ?? generateColor(index),
          };
        })
        .filter((series) => !hiddenSeriesNames.has(series.name));
    }
  }
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

/**
 * A widget that renders multi-metric charts for performance data.
 * It handles the layout of series editors, filters, and rendering the time-series chart itself.
 */
export function ChartWidget({
  onUpdate,
  widget,
  widgetId,
  filterColumns,
  isLoadingFilterColumns,
  globalFilters,
  dataSpecs,
}: ChartWidgetProps) {
  const isDistribution = useMemo(
    () =>
      getSafeChartType(widget.chartType) ===
      PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    [widget.chartType],
  );

  const [hiddenSeriesNames, setHiddenSeriesNames] = useState<Set<string>>(
    new Set(),
  );

  const handleToggleVisibility = useCallback((seriesName: string) => {
    setHiddenSeriesNames((prev) => {
      const next = new Set(prev);
      if (next.has(seriesName)) {
        next.delete(seriesName);
      } else {
        next.add(seriesName);
      }
      return next;
    });
  }, []);

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

  const handleWidgetAggregationUpdate = (
    newAgg: PerfChartSeries_PerfAggregationFunction,
  ) => {
    const updatedSeries =
      widget.series?.map((s) => ({
        ...s,
        aggregation: newAgg,
      })) ?? [];
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        series: updatedSeries,
      }),
    );
  };

  const handleWidgetGroupByUpdate = (newGroupBy: string) => {
    const newXAxis = GROUP_BY_CONFIG[newGroupBy]?.xAxis;

    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        xAxis: newXAxis,
      }),
    );
  };

  const currentGroupBy = useMemo(() => {
    if (!widget.xAxis) {
      return 'TIMESTAMP';
    }
    const granularity = widget.xAxis.granularity ?? 0;

    const numericGranularity =
      typeof granularity === 'string'
        ? perfXAxisConfig_GranularityFromJSON(granularity)
        : granularity;

    return getGroupByFromGranularity(numericGranularity);
  }, [widget.xAxis]);

  const currentAggregation = useMemo(() => {
    const seriesAgg = widget.series?.[0]?.aggregation;
    if (seriesAgg) {
      const numericAgg =
        typeof seriesAgg === 'string'
          ? perfChartSeries_PerfAggregationFunctionFromJSON(seriesAgg)
          : seriesAgg;
      return numericAgg ===
        PerfChartSeries_PerfAggregationFunction.PERF_AGGREGATION_FUNCTION_UNSPECIFIED
        ? PerfChartSeries_PerfAggregationFunction.MEAN
        : numericAgg;
    }
    return PerfChartSeries_PerfAggregationFunction.MEAN;
  }, [widget.series]);

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
              xAxis: widget.xAxis ?? DEFAULT_X_AXIS_CONFIG,
            }),
          }),
        ],
      }),
      widgetId,
    }),
    [globalFilters, dataSpecs, widgetId, widget],
  );

  const hasAtpTestFilter = useMemo(() => {
    const allFilters = [
      ...(globalFilters ?? []),
      ...(widget.filters ?? []),
      ...(widget.series?.flatMap((s) => s.filters ?? []) ?? []),
    ];
    return allFilters.some(
      (f) =>
        f.column === Column.ATP_TEST_NAME &&
        f.textInput?.defaultValue?.values?.[0],
    );
  }, [globalFilters, widget.filters, widget.series]);

  const hasEmptyMetricField = useMemo(() => {
    return widget.series?.some((s) => !s.metricField);
  }, [widget.series]);

  const {
    data: widgetResponse,
    isLoading: isWidgetLoading,
    isError: isWidgetError,
    error: widgetError,
  } = useFetchDashboardWidgetData(fetchRequest, {
    enabled:
      !!widgetId &&
      hasAtpTestFilter &&
      !hasEmptyMetricField &&
      (widget.series?.length ?? 0) > 0,
  });

  const chartSeries = useMemo(
    () =>
      getChartSeries(
        getSafeChartType(widget.chartType),
        widgetResponse,
        widget.series,
        hiddenSeriesNames,
      ),
    [widget.chartType, widgetResponse, widget.series, hiddenSeriesNames],
  );

  const xAxisBounds = useMemo(() => {
    let timeRangeStart: number | undefined;
    let timeRangeEnd: number | undefined;

    const timeFilters =
      globalFilters?.flatMap((f) => {
        if (f.column !== 'TIMESTAMP') return [];
        return parseSingleFilter(f, ['TIMESTAMP']);
      }) ?? [];

    timeFilters.forEach((f) => {
      if (
        f.operator === PerfFilterDefault_FilterOperator.GREATER_THAN_OR_EQUAL
      ) {
        timeRangeStart = Date.parse(f.value);
      }
      if (f.operator === PerfFilterDefault_FilterOperator.LESS_THAN_OR_EQUAL) {
        timeRangeEnd = Date.parse(f.value);
      }
    });

    const inPastFilter = globalFilters?.find((f) => {
      const rangeOp =
        f.range?.defaultValue?.filterOperator !== undefined
          ? perfFilterDefault_FilterOperatorFromJSON(
              f.range.defaultValue.filterOperator,
            )
          : undefined;
      return (
        f.column === 'TIMESTAMP' &&
        rangeOp === PerfFilterDefault_FilterOperator.IN_PAST
      );
    });

    if (inPastFilter && timeRangeStart !== undefined) {
      timeRangeEnd = Date.now();
    }

    let dataMin: number | undefined;
    let dataMax: number | undefined;

    chartSeries.forEach((s) => {
      s.data.forEach((p) => {
        const x = p.x;
        if (dataMin === undefined || x < dataMin) dataMin = x;
        if (dataMax === undefined || x > dataMax) dataMax = x;
      });
    });

    const minVal =
      timeRangeStart !== undefined && dataMin !== undefined
        ? Math.min(timeRangeStart, dataMin)
        : (timeRangeStart ?? dataMin);

    const maxVal =
      timeRangeEnd !== undefined && dataMax !== undefined
        ? Math.max(timeRangeEnd, dataMax)
        : (timeRangeEnd ?? dataMax);

    return { min: minVal, max: maxVal };
  }, [globalFilters, chartSeries]);

  const tooltipFormatter = useMemo(() => {
    return (params: ChartTooltipParam | ChartTooltipParam[]) => {
      const items = Array.isArray(params) ? params : [params];
      if (items.length === 0) return '';

      const firstItem = items[0];
      const xVal = firstItem.axisValue;
      let xDisplay = xVal;

      const xAxisDataKey = isDistribution
        ? widgetResponse?.invocationDistributionData?.xAxisDataKey
        : widgetResponse?.multiMetricChartData?.xAxisDataKey;

      const xAxisType = xAxisDataKey === Column.BUILD_ID ? 'value' : 'time';
      if (xAxisType === 'time' && typeof xVal === 'number') {
        const date = new Date(xVal);
        xDisplay = `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;
      }

      let result = `<strong>${xDisplay}</strong><br/>`;

      items.forEach((item) => {
        const val = item.data[1];
        const count = item.data[2];

        result += `${item.marker}${item.seriesName}: ${val.toLocaleString()}`;
        if (count !== undefined && count !== 0) {
          result += ` (n=${count})`;
        }
        result += '<br/>';
      });
      return result;
    };
  }, [isDistribution, widgetResponse]);

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
      <Box
        sx={{
          bgcolor: (theme) => theme.palette.action.hover,
          mx: -2,
          mt: -2,
          mb: 2,
        }}
      >
        <FilterEditor
          title="Widget Filters"
          filters={[...(widget.filters ?? [])]}
          onUpdateFilters={handleFiltersUpdate}
          dataSpecId={widget.dataSpecId}
          availableColumns={widgetFilterColumns}
          isLoadingColumns={isLoadingFilterColumns}
          globalFilters={globalFilters}
          uiStateOptions={{
            prefix: EditorUiKeyPrefix.WIDGET_FILTERS,
            key: widgetId,
            initialValue: true,
          }}
        />
        <Divider light />
        <ChartWidgetToolbar
          chartType={getSafeChartType(widget.chartType)}
          onChartTypeChange={(newChartType) => {
            onUpdate({
              ...widget,
              chartType: newChartType,
              effectiveChartType: newChartType,
            });
          }}
          currentGroupBy={currentGroupBy}
          onGroupByChange={handleWidgetGroupByUpdate}
          currentAggregation={currentAggregation}
          onAggregationChange={handleWidgetAggregationUpdate}
        />
        <Divider light />
      </Box>
      <Box
        sx={{
          position: 'relative',
          minHeight: (theme) => theme.spacing(50),
          mt: 2,
        }}
      >
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
            {widgetError?.message ?? COMMON_MESSAGES.UNKNOWN_ERROR}
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
                {!widget.series || widget.series.length === 0
                  ? COMMON_MESSAGES.METRIC_REQUIRED
                  : hasEmptyMetricField
                    ? COMMON_MESSAGES.EMPTY_METRIC_FIELD
                    : !hasAtpTestFilter
                      ? COMMON_MESSAGES.ATP_TEST_NAME_REQUIRED
                      : COMMON_MESSAGES.NO_DATA_FOUND}
              </Typography>
            </Box>
          )}
        {!isWidgetError && widgetResponse && hasData && (
          <TimeSeriesChart
            series={chartSeries}
            yAxisLabel="Value"
            xAxisType={
              (isDistribution
                ? widgetResponse?.invocationDistributionData?.xAxisDataKey
                : widgetResponse?.multiMetricChartData?.xAxisDataKey) ===
              Column.BUILD_ID
                ? 'value'
                : 'time'
            }
            chartType={isDistribution ? 'scatter' : 'line'}
            tooltipFormatter={tooltipFormatter}
            xAxisMin={xAxisBounds.min}
            xAxisMax={xAxisBounds.max}
          />
        )}
      </Box>
      <ChartSeriesEditor
        series={[...(widget.series ?? [])]}
        onUpdateSeries={handleSeriesUpdate}
        hiddenSeriesNames={hiddenSeriesNames}
        onToggleVisibility={handleToggleVisibility}
        dataSpecId={widget.dataSpecId}
        globalFilters={globalFilters}
        widgetFilters={widget.filters}
        filterColumns={filterColumns}
        isLoadingFilterColumns={isLoadingFilterColumns}
      />
    </Box>
  );
}
