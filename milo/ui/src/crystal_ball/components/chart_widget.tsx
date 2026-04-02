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
  Functions as FunctionsIcon,
  GroupWork as GroupWorkIcon,
} from '@mui/icons-material';
import {
  Alert,
  Box,
  CircularProgress,
  Divider,
  FormControl,
  MenuItem,
  Select,
  SelectChangeEvent,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';

import {
  ChartSeriesEditor,
  FilterEditor,
  TimeSeriesChart,
} from '@/crystal_ball/components';
import {
  Column,
  COMMON_MESSAGES,
  GOLDEN_RATIO_CONJUGATE,
  AGGREGATION_FUNCTION_LABELS,
  DEFAULT_X_AXIS_CONFIG,
  getGroupByFromGranularity,
  GROUP_BY_CONFIG,
  GROUP_BY_OPTIONS,
} from '@/crystal_ball/constants';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks';
import { COMPACT_ICON_SX, COMPACT_SELECT_SX } from '@/crystal_ball/styles';
import { isStringArray } from '@/crystal_ball/utils';
import {
  dataPointsToData,
  isDataPointsValid,
} from '@/crystal_ball/utils/widget_utils';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfChartSeries_PerfAggregationFunction,
  perfChartSeries_PerfAggregationFunctionFromJSON,
  PerfChartWidget,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
  perfXAxisConfig_GranularityFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

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
    const allFilters = [...(globalFilters ?? []), ...(widget.filters ?? [])];
    return allFilters.some(
      (f) =>
        f.column === Column.ATP_TEST_NAME &&
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

    return widgetResponse.multiMetricChartData.lines
      .map((line, index) => {
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
      })
      .filter((series) => !hiddenSeriesNames.has(series.name));
  }, [widgetResponse, widget.series, hiddenSeriesNames]);

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
        />
        <Divider light />
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            pl: 2,
            pr: 0.5,
            py: 0.5,
            gap: 1,
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <GroupWorkIcon sx={COMPACT_ICON_SX} />
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {COMMON_MESSAGES.GROUP_BY}
            </Typography>
          </Box>
          <FormControl size="small" variant="outlined">
            <Select
              id="widget-groupby-select"
              value={currentGroupBy}
              onChange={(e: SelectChangeEvent<string>) => {
                handleWidgetGroupByUpdate(e.target.value);
              }}
              inputProps={{ 'aria-label': 'Widget Group By' }}
              sx={COMPACT_SELECT_SX}
            >
              {GROUP_BY_OPTIONS.map((opt) => (
                <MenuItem key={opt.value} value={opt.value}>
                  {opt.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <Divider orientation="vertical" flexItem light />

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <FunctionsIcon sx={COMPACT_ICON_SX} />
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {COMMON_MESSAGES.AGGREGATE_BY}
            </Typography>
          </Box>
          <FormControl size="small" variant="outlined">
            <Select
              id="widget-aggregation-select"
              value={currentAggregation}
              onChange={(e) => {
                if (typeof e.target.value === 'number') {
                  handleWidgetAggregationUpdate(e.target.value);
                }
              }}
              inputProps={{ 'aria-label': 'Widget Aggregation' }}
              sx={COMPACT_SELECT_SX}
            >
              {Object.entries(AGGREGATION_FUNCTION_LABELS)
                .filter(([value]) => Number(value) !== 0)
                .map(([value, label]) => (
                  <MenuItem key={value} value={Number(value)}>
                    {label}
                  </MenuItem>
                ))}
            </Select>
          </FormControl>
        </Box>
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
            yAxisLabel="Value"
            xAxisType={
              widgetResponse?.multiMetricChartData?.xAxisDataKey ===
              Column.BUILD_ID
                ? 'value'
                : 'time'
            }
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
