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
  Card,
  CardContent,
  CardHeader,
  Checkbox,
  Chip,
  CircularProgress,
  Divider,
  FormControlLabel,
  FormGroup,
  Paper,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
  useTheme,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  TooltipProps,
  XAxis,
  YAxis,
} from 'recharts';

import { colors } from '@/fleet/theme/colors';
import { TrendlineGrouping } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  buildTimeAxisTicks,
  buildTrendsChartRows,
  buildValueAxisTicks,
  BUCKET_INTERVAL_MS,
  computeMaxYScale,
  DEFAULT_VISIBLE_SERIES,
  findFreeColorSlot,
  formatPercentTick,
  formatTickLabel,
  formatTooltipDate,
  MAX_VISIBLE_SERIES,
  PALETTE,
  SeriesColorSlots,
  TrendsChartRow,
} from './trends_chart_data';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

const CHART_HEIGHT = 300;

/** A series that is currently plotted, and the color it owns. */
interface PlottedSeries {
  readonly name: string;
  readonly color: string;
}

interface TrendsTooltipProps extends TooltipProps<number, string> {
  readonly metricLabel: string;
}

type TooltipEntry = NonNullable<
  TooltipProps<number, string>['payload']
>[number];

/**
 * Whether a payload entry carries a reading. Series with no data in the
 * hovered bucket are still present in the payload, with no value.
 */
const hasReading = (
  entry: TooltipEntry,
): entry is TooltipEntry & { value: number } => typeof entry.value === 'number';

const TrendsTooltip = ({
  active,
  payload,
  label,
  metricLabel,
}: TrendsTooltipProps) => {
  if (!active || !payload?.length || typeof label !== 'number') return null;

  // A series with no reading in this bucket is left out rather than shown as
  // zero: it had no data then, which is not the same as having measured zero.
  const entries = payload.filter(hasReading);
  if (entries.length === 0) return null;

  return (
    <Paper
      elevation={3}
      data-testid="trends-chart-tooltip"
      sx={{
        p: 1,
        minWidth: 170,
        maxWidth: 230,
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 1.5,
      }}
    >
      <Typography
        variant="caption"
        sx={{
          fontWeight: 'bold',
          color: 'text.secondary',
          display: 'block',
          mb: 0.5,
        }}
      >
        {formatTooltipDate(label)}
      </Typography>

      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
        {entries.map((entry) => (
          <Box
            key={entry.name}
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 1.25,
            }}
          >
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.75,
                minWidth: 0,
              }}
            >
              <Box
                sx={{
                  width: 7,
                  height: 7,
                  borderRadius: '50%',
                  bgcolor: entry.color,
                  flexShrink: 0,
                }}
              />
              <Typography
                variant="caption"
                noWrap
                sx={{ fontWeight: 600, color: 'text.primary' }}
              >
                {entry.name}
              </Typography>
            </Box>
            <Typography
              variant="caption"
              sx={{
                fontWeight: 'bold',
                color: 'text.primary',
                flexShrink: 0,
              }}
            >
              {(entry.value * 100).toFixed(1)}% {metricLabel}
            </Typography>
          </Box>
        ))}
      </Box>
    </Paper>
  );
};

export const HistoricalAvailabilityTrendsChart = () => {
  const theme = useTheme();
  const [viewBy, setViewBy] = useState<TrendlineGrouping>(
    TrendlineGrouping.GROUP_BY_OVERALL,
  );
  const [colorSlots, setColorSlots] = useState<SeriesColorSlots>({});

  const queryRequest = useMemo(
    () => ({
      grouping: viewBy,
      startTime: undefined,
      endTime: undefined,
      filter: '',
    }),
    [viewBy],
  );

  const { data, isLoading, isError, error } =
    useFleetAvailabilityTrends(queryRequest);

  const seriesList = useMemo(() => data?.series ?? [], [data?.series]);

  // The default selection is seeded once per grouping, as soon as that
  // grouping's first response arrives. It deliberately does not re-run on
  // later renders: `seriesList` is a new array after every background refetch,
  // and reseeding from it would both discard the user's selection and reassign
  // colors.
  const seededGrouping = useRef<TrendlineGrouping | null>(null);

  useEffect(() => {
    if (seededGrouping.current === viewBy) return;

    // Overall is a single unselectable series, so it needs no slots.
    if (viewBy === TrendlineGrouping.GROUP_BY_OVERALL) {
      seededGrouping.current = viewBy;
      setColorSlots({});
      return;
    }

    // Wait for the first response before choosing defaults.
    if (seriesList.length === 0) return;

    seededGrouping.current = viewBy;
    const initial: SeriesColorSlots = {};
    seriesList.slice(0, DEFAULT_VISIBLE_SERIES).forEach((series, idx) => {
      initial[series.name] = idx;
    });
    setColorSlots(initial);
  }, [viewBy, seriesList]);

  const handleToggleSeries = (name: string) => {
    setColorSlots((prev) => {
      if (name in prev) {
        const next = { ...prev };
        delete next[name];
        return next;
      }

      const slot = findFreeColorSlot(prev);
      // Every color is in use. The checkbox is disabled in this state, so this
      // is only reachable via a race; ignoring it is better than plotting two
      // series in the same color.
      if (slot === undefined) return prev;

      return { ...prev, [name]: slot };
    });
  };

  const activeSeries = useMemo(() => {
    if (viewBy === TrendlineGrouping.GROUP_BY_OVERALL) {
      return seriesList;
    }
    return seriesList.filter((series) => series.name in colorSlots);
  }, [viewBy, seriesList, colorSlots]);

  const isChartFull = Object.keys(colorSlots).length >= MAX_VISIBLE_SERIES;

  const rows = useMemo(
    () => buildTrendsChartRows(activeSeries),
    [activeSeries],
  );

  const plotted: PlottedSeries[] = useMemo(
    () =>
      activeSeries.map((series) => ({
        name: series.name,
        // Overall has no selector and therefore no slot, so it falls back to
        // the first palette entry.
        color: PALETTE[colorSlots[series.name] ?? 0],
      })),
    [activeSeries, colorSlots],
  );

  const maxYScale = useMemo(
    () => computeMaxYScale(activeSeries),
    [activeSeries],
  );
  const yTicks = useMemo(() => buildValueAxisTicks(maxYScale), [maxYScale]);
  const xTicks = useMemo(() => buildTimeAxisTicks(rows), [rows]);

  // A single bucket would collapse the time axis to a point, so it is given an
  // hour of breathing room on either side.
  const xDomain = useMemo((): [number, number] | undefined => {
    if (rows.length === 0) return undefined;
    const first = rows[0].timestampMs;
    const last = rows[rows.length - 1].timestampMs;
    return first === last
      ? [first - BUCKET_INTERVAL_MS / 2, last + BUCKET_INTERVAL_MS / 2]
      : [first, last];
  }, [rows]);

  const axisTickStyle = useMemo(
    () => ({
      fill: colors.grey[600],
      fontSize: theme.typography.caption.fontSize,
      fontFamily: theme.typography.caption.fontFamily,
    }),
    [theme],
  );

  const metricLabel =
    viewBy === TrendlineGrouping.GROUP_BY_MODEL ? 'Availability' : 'Health';

  const subtitle = useMemo(() => {
    switch (viewBy) {
      case TrendlineGrouping.GROUP_BY_MODEL:
        return 'Historical availability trendlines across models.';
      case TrendlineGrouping.GROUP_BY_POOL:
        return 'Historical health percentage trendlines across pools.';
      case TrendlineGrouping.GROUP_BY_OVERALL:
      default:
        return 'Overall fleet health percentage trendline over the last 72 hours.';
    }
  }, [viewBy]);

  return (
    <Card
      variant="outlined"
      sx={{
        borderRadius: 2,
        width: '100%',
      }}
    >
      <CardHeader
        title={
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: 2,
              width: '100%',
            }}
          >
            <Box>
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 1.5,
                  mb: 0.5,
                }}
              >
                <Typography
                  variant="h6"
                  component="h2"
                  sx={{ fontWeight: 'bold' }}
                >
                  Fleet Availability & Health Trends
                </Typography>
                <Chip
                  label="Last 72h"
                  size="small"
                  variant="outlined"
                  sx={{
                    fontWeight: 600,
                    fontSize: 12,
                    height: 22,
                    color: 'text.secondary',
                    borderColor: 'divider',
                  }}
                />
              </Box>
              <Typography variant="caption" color="text.secondary">
                {subtitle}
              </Typography>
            </Box>

            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1.5,
                ml: 'auto',
              }}
            >
              <ToggleButtonGroup
                value={viewBy}
                exclusive
                onChange={(_, val: TrendlineGrouping | null) => {
                  if (val !== null) setViewBy(val);
                }}
                size="small"
                aria-label="Trendline grouping tabs"
                sx={{
                  height: 28,
                  '& .MuiToggleButton-root': {
                    textTransform: 'none',
                    px: 1.5,
                    py: 0.25,
                    fontSize: 13,
                    height: 28,
                  },
                }}
              >
                <ToggleButton value={TrendlineGrouping.GROUP_BY_OVERALL}>
                  Overall
                </ToggleButton>
                <ToggleButton value={TrendlineGrouping.GROUP_BY_MODEL}>
                  By Model
                </ToggleButton>
                <ToggleButton value={TrendlineGrouping.GROUP_BY_POOL}>
                  By Pool
                </ToggleButton>
              </ToggleButtonGroup>
            </Box>
          </Box>
        }
      />
      <Divider />
      <CardContent sx={{ p: 2 }}>
        {isLoading && (
          <Box
            data-testid="trends-loading"
            sx={{
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              height: CHART_HEIGHT,
            }}
          >
            <CircularProgress size={36} />
          </Box>
        )}

        {isError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            Failed to load historical trendlines:{' '}
            {error?.message || 'Unknown error'}
          </Alert>
        )}

        {!isLoading && !isError && (
          <Box
            sx={
              viewBy === TrendlineGrouping.GROUP_BY_OVERALL
                ? { width: '100%' }
                : {
                    display: 'grid',
                    gridTemplateColumns: {
                      xs: '1fr',
                      sm: '220px 1fr',
                    },
                    gap: 3,
                  }
            }
          >
            {/* Multi-series selector sidebar for By Model and By Pool */}
            {viewBy !== TrendlineGrouping.GROUP_BY_OVERALL && (
              <Box
                sx={{
                  maxHeight: 280,
                  overflowY: 'auto',
                  pr: 1,
                  '&::-webkit-scrollbar': { width: '4px' },
                  '&::-webkit-scrollbar-thumb': {
                    backgroundColor: 'divider',
                    borderRadius: '4px',
                  },
                }}
              >
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 'bold', mb: 1, display: 'block' }}
                >
                  SELECT SERIES TO DISPLAY:
                </Typography>

                {isChartFull && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ mb: 1, display: 'block' }}
                  >
                    Showing {MAX_VISIBLE_SERIES} series, the maximum. Clear one
                    to add another.
                  </Typography>
                )}

                <FormGroup>
                  {seriesList.map((series) => {
                    const slot = colorSlots[series.name];
                    const isChecked = slot !== undefined;
                    return (
                      <FormControlLabel
                        key={series.name}
                        disabled={!isChecked && isChartFull}
                        control={
                          <Checkbox
                            size="small"
                            checked={isChecked}
                            onChange={() => handleToggleSeries(series.name)}
                            sx={
                              isChecked
                                ? {
                                    color: PALETTE[slot],
                                    '&.Mui-checked': {
                                      color: PALETTE[slot],
                                    },
                                  }
                                : undefined
                            }
                          />
                        }
                        label={
                          <Typography variant="body2">{series.name}</Typography>
                        }
                      />
                    );
                  })}
                </FormGroup>
              </Box>
            )}

            {/* `minWidth: 0` lets this grid column shrink. Without it the
                track's minimum is the chart's own width, so the chart can
                grow with the window but never shrink back. */}
            <Box sx={{ width: '100%', minWidth: 0 }}>
              {seriesList.length === 0 ? (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    height: CHART_HEIGHT,
                    color: 'text.secondary',
                  }}
                >
                  <Typography variant="body2">
                    No trend data available for the last 72 hours.
                  </Typography>
                </Box>
              ) : activeSeries.length === 0 ? (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    height: CHART_HEIGHT,
                    color: 'text.secondary',
                  }}
                >
                  <Typography variant="body2">
                    No series selected. Please select at least one series from
                    the list.
                  </Typography>
                </Box>
              ) : (
                <Box
                  data-testid="trends-svg-chart"
                  role="img"
                  aria-label="Fleet historical availability and health trendlines chart"
                  sx={{ width: '100%' }}
                >
                  <ResponsiveContainer width="100%" height={CHART_HEIGHT}>
                    <LineChart
                      // Trust that the chart will not modify readonly data.
                      data={rows as TrendsChartRow[]}
                      margin={{ top: 16, right: 24, bottom: 8, left: 0 }}
                    >
                      <CartesianGrid
                        vertical={false}
                        stroke={colors.grey[300]}
                        strokeDasharray="2 2"
                      />
                      <XAxis
                        type="number"
                        scale="time"
                        dataKey="timestampMs"
                        domain={xDomain}
                        ticks={xTicks}
                        // The labels are wide, so recharts is allowed to drop
                        // any that would collide, but never the two that
                        // anchor the range.
                        interval="preserveStartEnd"
                        minTickGap={24}
                        tickFormatter={formatTickLabel}
                        tick={axisTickStyle}
                        tickLine={false}
                        stroke={colors.grey[300]}
                      />
                      <YAxis
                        domain={[0, maxYScale]}
                        ticks={yTicks}
                        tickFormatter={formatPercentTick}
                        tick={axisTickStyle}
                        tickLine={false}
                        axisLine={false}
                        width={48}
                      />
                      <Tooltip
                        isAnimationActive={false}
                        cursor={{
                          stroke: colors.grey[500],
                          strokeWidth: 1,
                          strokeDasharray: '3 3',
                        }}
                        content={<TrendsTooltip metricLabel={metricLabel} />}
                      />
                      {plotted.map(({ name, color }) => (
                        // Keyed by name, never by index: an index key lets
                        // React reuse the node, so deselecting a series makes
                        // its neighbour inherit the wrong data.
                        <Line
                          key={name}
                          name={name}
                          data-testid={`series-line-${name}`}
                          // A function reads the value rather than a string
                          // path, because a series name containing a dot would
                          // otherwise be read as a nested lookup.
                          dataKey={(row: TrendsChartRow) =>
                            row.values[name] ?? null
                          }
                          type="linear"
                          stroke={color}
                          strokeWidth={2.2}
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          // A bucket with no reading is a break in the line,
                          // not a point to interpolate through.
                          connectNulls={false}
                          // Every reading is marked, because a reading whose
                          // neighbouring buckets are empty has no segment to
                          // draw and would otherwise not appear at all. Dots
                          // with no coordinate, i.e. the empty buckets, are
                          // skipped by recharts rather than drawn at zero.
                          dot={{
                            r: 2.5,
                            fill: color,
                            stroke: colors.white,
                            strokeWidth: 1.5,
                          }}
                          activeDot={{
                            r: 4,
                            strokeWidth: 1.5,
                            stroke: colors.white,
                          }}
                          // Background refetches would otherwise make the
                          // chart redraw itself for no visible reason.
                          isAnimationActive={false}
                        />
                      ))}
                    </LineChart>
                  </ResponsiveContainer>
                </Box>
              )}
            </Box>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};
