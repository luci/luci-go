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
  CalendarMonth as CalendarMonthIcon,
  Functions as FunctionsIcon,
  TrendingDown as TrendingDownIcon,
  TrendingFlat as TrendingFlatIcon,
  TrendingUp as TrendingUpIcon,
} from '@mui/icons-material';
import {
  Box,
  Checkbox,
  CircularProgress,
  Divider,
  FormControl,
  InputLabel,
  ListSubheader,
  MenuItem,
  OutlinedInput,
  Paper,
  Select,
  SelectChangeEvent,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { ChartSeriesEditor } from '@/crystal_ball/components';
import { Column } from '@/crystal_ball/constants';
import { useFetchDashboardWidgetData } from '@/crystal_ball/hooks';
import { generateColor, getTrendInfo } from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  PerfChartSeries,
  PerfChartSeries_PerfAggregationFunction,
  perfChartSeries_PerfAggregationFunctionFromJSON,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Props for the PeriodComparisonWidget component.
 */
export interface PeriodComparisonWidgetProps {
  widget: PerfChartWidget;
  dashboardName: string;
  widgetId: string;
  globalFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
  dataSpecs?: { [key: string]: PerfDataSpec };
  onUpdate: (updatedWidget: PerfChartWidget) => void;
}

/**
 * Determines color coding for a given metric field based on typical polarity.
 */
function getDiffColor(
  netChange: number,
  metricField: string,
): { color: string; icon: React.ReactNode } {
  const trendInfo = getTrendInfo(netChange, metricField);
  if (trendInfo.trend === 'flat') {
    return {
      color: 'text.secondary',
      icon: <TrendingFlatIcon color="action" fontSize="small" />,
    };
  }

  const iconColor = trendInfo.color === 'error.main' ? 'error' : 'success';
  const IconComponent =
    trendInfo.trend === 'up' ? TrendingUpIcon : TrendingDownIcon;

  return {
    color: trendInfo.color,
    icon: <IconComponent color={iconColor} fontSize="small" />,
  };
}

/**
 * Displays a premium period comparison table for performance metrics.
 */
export function PeriodComparisonWidget({
  widget,
  widgetId,
  globalFilters,
  filterColumns,
  isLoadingFilterColumns,
  dataSpecs,
  onUpdate,
}: PeriodComparisonWidgetProps): React.ReactElement {
  const handleSeriesUpdate = (updatedSeries: PerfChartSeries[]) => {
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        series: updatedSeries,
      }),
    );
  };

  const currentAggregations = useMemo(() => {
    const config = widget.periodComparisonChartConfig;
    if (config?.aggregations?.length) {
      return config.aggregations.map((a) =>
        perfChartSeries_PerfAggregationFunctionFromJSON(a),
      );
    }
    return [PerfChartSeries_PerfAggregationFunction.MEAN];
  }, [widget.periodComparisonChartConfig]);

  const baselineTime = useMemo(() => {
    const start =
      widget.periodComparisonChartConfig?.baselineTimeRange?.startTime;
    const end = widget.periodComparisonChartConfig?.baselineTimeRange?.endTime;
    return {
      start: start ? DateTime.fromISO(start).toUTC() : null,
      end: end ? DateTime.fromISO(end).toUTC() : null,
    };
  }, [widget.periodComparisonChartConfig]);

  const comparisonTime = useMemo(() => {
    const start =
      widget.periodComparisonChartConfig?.comparisonTimeRange?.startTime;
    const end =
      widget.periodComparisonChartConfig?.comparisonTimeRange?.endTime;
    return {
      start: start ? DateTime.fromISO(start).toUTC() : null,
      end: end ? DateTime.fromISO(end).toUTC() : null,
    };
  }, [widget.periodComparisonChartConfig]);

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

  const hasTimeIntervalsSet = useMemo(() => {
    return Boolean(
      baselineTime.start &&
        baselineTime.end &&
        comparisonTime.start &&
        comparisonTime.end,
    );
  }, [baselineTime, comparisonTime]);

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
              chartType: PerfChartWidget_ChartType.PERIOD_COMPARISON,
              periodComparisonChartConfig: {
                aggregations: [...currentAggregations],
                baselineTimeRange:
                  widget.periodComparisonChartConfig?.baselineTimeRange,
                comparisonTimeRange:
                  widget.periodComparisonChartConfig?.comparisonTimeRange,
              },
            }),
          }),
        ],
      }),
      widgetId,
    }),
    [globalFilters, dataSpecs, widgetId, widget, currentAggregations],
  );

  const {
    data: responseData,
    isLoading,
    error,
  } = useFetchDashboardWidgetData(fetchRequest, {
    enabled:
      !!widgetId &&
      (widget.series?.length ?? 0) > 0 &&
      hasAtpTestFilter &&
      !hasEmptyMetricField &&
      hasTimeIntervalsSet,
  });

  const periodComparisonSeries =
    responseData?.periodComparisonData?.series ?? [];

  const updateBaselineInterval = (
    start: DateTime | null,
    end: DateTime | null,
  ) => {
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        periodComparisonChartConfig: {
          ...widget.periodComparisonChartConfig,
          baselineTimeRange: {
            startTime: start ? (start.toISO() ?? '') : '',
            endTime: end ? (end.toISO() ?? '') : '',
          },
        },
      }),
    );
  };

  const updateComparisonInterval = (
    start: DateTime | null,
    end: DateTime | null,
  ) => {
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        periodComparisonChartConfig: {
          ...widget.periodComparisonChartConfig,
          comparisonTimeRange: {
            startTime: start ? (start.toISO() ?? '') : '',
            endTime: end ? (end.toISO() ?? '') : '',
          },
        },
      }),
    );
  };

  const handleAggregationsChange = (
    event: SelectChangeEvent<PerfChartSeries_PerfAggregationFunction[]>,
  ) => {
    const value = event.target.value;
    const valArray = typeof value === 'string' ? value.split(',') : value;
    const aggregations = valArray.map((v) =>
      perfChartSeries_PerfAggregationFunctionFromJSON(v),
    );
    onUpdate(
      PerfChartWidget.fromPartial({
        ...widget,
        periodComparisonChartConfig: {
          ...widget.periodComparisonChartConfig,
          aggregations,
        },
      }),
    );
  };

  const warningMessage = useMemo(() => {
    if (!hasAtpTestFilter) {
      return 'Please specify a valid test filter (e.g. atp_test_name) to fetch data.';
    } else if (hasEmptyMetricField) {
      return 'Please select a metric field in the configuration.';
    } else if (!hasTimeIntervalsSet) {
      return 'Please configure both Baseline and Comparison time intervals above.';
    } else if (periodComparisonSeries.length === 0) {
      return 'No comparative performance data found for the selected periods.';
    }
    return null;
  }, [
    hasAtpTestFilter,
    hasEmptyMetricField,
    hasTimeIntervalsSet,
    periodComparisonSeries.length,
  ]);

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 2.5,
        height: '100%',
      }}
    >
      {/* Configuration Control Panel */}
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', lg: '2fr 2fr 1.5fr' },
          gap: 2,
          p: 2,
          borderRadius: 2,
          bgcolor: 'action.hover',
          border: '1px dashed',
          borderColor: 'divider',
          alignItems: 'flex-start',
        }}
      >
        {/* Baseline Time Interval Panel */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <CalendarMonthIcon color="primary" fontSize="small" />
            <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
              Baseline Period (UTC)
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', gap: 1.5 }}>
            <DateTimePicker
              label="From"
              timezone="UTC"
              value={baselineTime.start}
              onChange={(val) => updateBaselineInterval(val, baselineTime.end)}
              slotProps={{
                textField: { size: 'small', fullWidth: true },
                field: { clearable: true },
              }}
            />
            <DateTimePicker
              label="To"
              timezone="UTC"
              value={baselineTime.end}
              onChange={(val) =>
                updateBaselineInterval(baselineTime.start, val)
              }
              slotProps={{
                textField: { size: 'small', fullWidth: true },
                field: { clearable: true },
              }}
            />
          </Box>
        </Box>

        {/* Comparison Time Interval Panel */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <CalendarMonthIcon color="secondary" fontSize="small" />
            <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
              Comparison Period (UTC)
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', gap: 1.5 }}>
            <DateTimePicker
              label="From"
              timezone="UTC"
              value={comparisonTime.start}
              onChange={(val) =>
                updateComparisonInterval(val, comparisonTime.end)
              }
              slotProps={{
                textField: { size: 'small', fullWidth: true },
                field: { clearable: true },
              }}
            />
            <DateTimePicker
              label="To"
              timezone="UTC"
              value={comparisonTime.end}
              onChange={(val) =>
                updateComparisonInterval(comparisonTime.start, val)
              }
              slotProps={{
                textField: { size: 'small', fullWidth: true },
                field: { clearable: true },
              }}
            />
          </Box>
        </Box>

        {/* Aggregations Panel */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <FunctionsIcon color="action" fontSize="small" />
            <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
              Aggregations
            </Typography>
          </Box>
          <FormControl size="small" fullWidth>
            <InputLabel id="aggregations-select-label">Functions</InputLabel>
            <Select
              labelId="aggregations-select-label"
              multiple
              value={currentAggregations}
              onChange={handleAggregationsChange}
              input={<OutlinedInput label="Functions" />}
              renderValue={(selected) => {
                if (!Array.isArray(selected)) {
                  return '';
                }
                return selected
                  .map((value) => {
                    return (
                      Object.entries(
                        PerfChartSeries_PerfAggregationFunction,
                      ).find(
                        ([k, v]) => isNaN(Number(k)) && v === value,
                      )?.[0] ?? String(value)
                    );
                  })
                  .join(', ');
              }}
            >
              <ListSubheader
                sx={{
                  fontWeight: (theme) => theme.typography.fontWeightBold,
                  fontSize: (theme) => theme.typography.caption.fontSize,
                  lineHeight: (theme) => theme.typography.pxToRem(32),
                  color: 'text.secondary',
                  backgroundColor: 'background.paper',
                }}
              >
                SELECT FUNCTIONS
              </ListSubheader>
              {Object.keys(PerfChartSeries_PerfAggregationFunction)
                .filter(
                  (k) =>
                    k !== 'PERF_AGGREGATION_FUNCTION_UNSPECIFIED' &&
                    isNaN(Number(k)),
                )
                .map((agg) => {
                  const aggVal =
                    perfChartSeries_PerfAggregationFunctionFromJSON(agg);
                  const isChecked =
                    Array.isArray(currentAggregations) &&
                    currentAggregations.includes(aggVal);

                  return (
                    <MenuItem
                      key={agg}
                      value={aggVal}
                      sx={{
                        fontSize: (theme) => theme.typography.body2.fontSize,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        py: 0.5,
                      }}
                    >
                      <Box sx={{ display: 'flex', alignItems: 'center' }}>
                        <Checkbox
                          checked={isChecked}
                          size="small"
                          sx={{ p: 0.5, mr: 1 }}
                        />
                        <span>{agg}</span>
                      </Box>
                    </MenuItem>
                  );
                })}
            </Select>
          </FormControl>
        </Box>
      </Box>

      <Divider />

      {/* Data Display / Render Layout */}
      {isLoading && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            py: 6,
          }}
        >
          <CircularProgress size={40} />
        </Box>
      )}

      {error && (
        <Paper
          variant="outlined"
          sx={{ p: 3, bgcolor: 'error.light', borderColor: 'error.main' }}
        >
          <Typography color="error.contrastText">
            Failed to fetch period comparison data.
          </Typography>
        </Paper>
      )}

      {!isLoading && !error && warningMessage && (
        <Paper variant="outlined" sx={{ p: 3, textAlign: 'center' }}>
          <Typography color="text.secondary">{warningMessage}</Typography>
        </Paper>
      )}

      {!isLoading && !error && !warningMessage && (
        <TableContainer component={Paper} variant="outlined">
          <Table
            sx={{ minWidth: 650 }}
            aria-label="Period comparison metrics table"
          >
            <TableHead sx={{ bgcolor: 'action.hover' }}>
              <TableRow>
                <TableCell>
                  <strong>Metric Line</strong>
                </TableCell>
                <TableCell>
                  <strong>Aggregation</strong>
                </TableCell>
                <TableCell align="right">
                  <strong>Baseline Value</strong>
                </TableCell>
                <TableCell align="right">
                  <strong>Comparison Value</strong>
                </TableCell>
                <TableCell align="right">
                  <strong>Net Change</strong>
                </TableCell>
                <TableCell align="right">
                  <strong>% Change</strong>
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {periodComparisonSeries
                .filter((series) => {
                  const configuredSeries = widget.series?.find(
                    (s) =>
                      s.id === series.seriesId ||
                      s.displayName === series.legendLabel ||
                      s.metricField === series.metricField,
                  );
                  return !(configuredSeries?.hidden ?? false);
                })
                .map((series, seriesIdx) => {
                  const configuredSeries = widget.series?.find(
                    (s) =>
                      s.id === series.seriesId ||
                      s.displayName === series.legendLabel ||
                      s.metricField === series.metricField,
                  );
                  const seriesColor = configuredSeries?.color
                    ? configuredSeries.color
                    : generateColor(seriesIdx);

                  return (series.aggregationData ?? []).map((agg, idx) => {
                    const netChange = agg.netChange ?? 0;
                    const { color, icon } = getDiffColor(
                      netChange,
                      series.metricField,
                    );
                    const baseVal = agg.baselineStats?.value ?? 0;
                    const compVal = agg.comparisonStats?.value ?? 0;

                    let isPositive = netChange > 0;
                    let pctChange = '0.00%';

                    if (
                      agg.percentageChange !== undefined &&
                      agg.percentageChange !== null &&
                      !Number.isNaN(agg.percentageChange) &&
                      Number.isFinite(agg.percentageChange)
                    ) {
                      pctChange = (agg.percentageChange * 100).toFixed(2) + '%';
                      isPositive = agg.percentageChange > 0;
                    } else {
                      if (baseVal !== 0) {
                        const pct = (compVal - baseVal) / baseVal;
                        pctChange = (pct * 100).toFixed(2) + '%';
                        isPositive = pct > 0;
                      } else if (compVal !== 0) {
                        pctChange = compVal > 0 ? '100.00%*' : '-100.00%*';
                        isPositive = compVal > 0;
                      }
                    }

                    return (
                      <TableRow
                        key={`${series.seriesId}-${agg.aggregationType}`}
                        sx={{
                          '&:last-child td, &:last-child th': { border: 0 },
                        }}
                      >
                        {idx === 0 && (
                          <TableCell
                            rowSpan={series.aggregationData?.length}
                            component="th"
                            scope="row"
                          >
                            <Box
                              sx={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: 1.5,
                              }}
                            >
                              <Box
                                sx={{
                                  width: 12,
                                  height: 12,
                                  borderRadius: '50%',
                                  bgcolor: seriesColor,
                                  flexShrink: 0,
                                }}
                              />
                              <Box>
                                <Typography
                                  variant="body2"
                                  sx={{ fontWeight: 'bold' }}
                                >
                                  {series.legendLabel}
                                </Typography>
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                  display="block"
                                >
                                  {series.metricField}
                                </Typography>
                              </Box>
                            </Box>
                          </TableCell>
                        )}
                        <TableCell>{agg.aggregationType}</TableCell>
                        <TableCell align="right">
                          {agg.baselineStats?.value?.toLocaleString(undefined, {
                            minimumFractionDigits: 2,
                            maximumFractionDigits: 2,
                          }) ?? '-'}
                        </TableCell>
                        <TableCell align="right">
                          {agg.comparisonStats?.value?.toLocaleString(
                            undefined,
                            {
                              minimumFractionDigits: 2,
                              maximumFractionDigits: 2,
                            },
                          ) ?? '-'}
                        </TableCell>
                        <TableCell
                          align="right"
                          sx={{ color, fontWeight: 'medium' }}
                        >
                          <Box
                            sx={{
                              display: 'flex',
                              alignItems: 'center',
                              justifyContent: 'flex-end',
                              gap: 0.5,
                            }}
                          >
                            {icon}
                            {netChange > 0 ? '+' : ''}
                            {netChange.toLocaleString(undefined, {
                              minimumFractionDigits: 2,
                              maximumFractionDigits: 2,
                            })}
                          </Box>
                        </TableCell>
                        <TableCell
                          align="right"
                          sx={{ color, fontWeight: 'medium' }}
                        >
                          {isPositive && pctChange !== '0.00%' ? '+' : ''}
                          {pctChange}
                        </TableCell>
                      </TableRow>
                    );
                  });
                })}
            </TableBody>
          </Table>
        </TableContainer>
      )}
      <ChartSeriesEditor
        series={[...(widget.series ?? [])]}
        onUpdateSeries={handleSeriesUpdate}
        dataSpecId={widget.dataSpecId}
        globalFilters={globalFilters}
        widgetFilters={widget.filters}
        filterColumns={filterColumns}
        isLoadingFilterColumns={isLoadingFilterColumns}
      />
    </Box>
  );
}
