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
import {
  memo,
  SVGProps,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
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
import {
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { BaselineChip } from './baseline_chip';
import {
  buildTimeAxisTicks,
  buildTrendsChartRows,
  buildValueAxisTicks,
  BUCKET_INTERVAL_MS,
  computeMaxYScale,
  DEFAULT_VISIBLE_SERIES_MODEL,
  DEFAULT_VISIBLE_SERIES_POOL,
  findFreeColorSlot,
  findNearestReading,
  formatPercentTick,
  formatTickLabel,
  formatTooltipDate,
  getSeriesColor,
  MAX_TOOLTIP_ROWS,
  selectTooltipRows,
  SeriesColorSlots,
  sortPoolSeries,
  TrendsChartRow,
  valueAtCursor,
} from './trends_chart_data';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

const CHART_HEIGHT = 300;

/** Opacity of the lines that are not hovered while another one is. */
const FADED_OPACITY = 0.2;

/**
 * Layout shared by the main chart and the highlight layer drawn over it. Both
 * must place the plot area at exactly the same pixels, so neither may size
 * its axes from content.
 */
const CHART_MARGIN = { top: 16, right: 24, bottom: 0, left: 0 } as const;
const TIME_AXIS_HEIGHT = 30;
const VALUE_AXIS_WIDTH = 48;

const LINE_WIDTH = 2.2;
const HIGHLIGHTED_LINE_WIDTH = 3.2;

/**
 * Lifts the tooltip above the highlight layer. The layer is positioned and
 * comes later in the DOM, so without this the hovered line paints over the
 * tooltip while the faded lines, which live in the main chart, sit under it.
 */
const TOOLTIP_WRAPPER_STYLE = { zIndex: 1 } as const;

/** A series that is currently plotted, and the color it owns. */
interface PlottedSeries {
  readonly name: string;
  readonly color: string;
}

interface TrendsTooltipProps extends TooltipProps<number, string> {
  readonly metricLabel: string;
  /** The top of the value axis, needed to turn the cursor height into a value. */
  readonly maxYScale: number;
  /**
   * Called with the series under the cursor, or null when there is none, so
   * the chart can emphasize that line. Must be referentially stable.
   */
  readonly onHoveredSeriesChange?: (name: string | null) => void;
}

type TooltipEntry = NonNullable<
  TooltipProps<number, string>['payload']
>[number];

/** A payload entry that carries a reading in the hovered bucket. */
interface TooltipRow {
  readonly name: string;
  readonly value: number;
  readonly color: string | undefined;
}

/**
 * Keeps only the payload entries that carry a reading. Series with no data in
 * the hovered bucket are still present in the payload, with no value; they are
 * left out rather than shown as zero, because having no data is not the same
 * as having measured zero.
 */
const toTooltipRows = (payload: readonly TooltipEntry[]): TooltipRow[] =>
  payload.flatMap((entry) =>
    typeof entry.value === 'number' && entry.name !== undefined
      ? [{ name: String(entry.name), value: entry.value, color: entry.color }]
      : [],
  );

interface TooltipRowViewProps {
  readonly row: TooltipRow;
  readonly metricLabel: string;
  readonly isHovered?: boolean;
}

const TooltipRowView = ({
  row,
  metricLabel,
  isHovered = false,
}: TooltipRowViewProps) => {
  const textSx = {
    fontWeight: isHovered ? 'bold' : 400,
    color: isHovered ? 'text.primary' : 'text.secondary',
  };
  return (
    <Box
      data-testid={
        isHovered ? 'trends-tooltip-hovered-row' : 'trends-tooltip-row'
      }
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
            width: isHovered ? 9 : 7,
            height: isHovered ? 9 : 7,
            borderRadius: '50%',
            bgcolor: row.color,
            flexShrink: 0,
          }}
        />
        <Typography variant="caption" noWrap title={row.name} sx={textSx}>
          {row.name}
        </Typography>
      </Box>
      <Typography variant="caption" sx={{ ...textSx, flexShrink: 0 }}>
        {(row.value * 100).toFixed(1)}% {metricLabel}
      </Typography>
    </Box>
  );
};

interface MoreSeriesProps {
  readonly count: number;
  readonly testId: string;
}

/** The "+N more series" line for rows cut off above or below the window. */
const MoreSeries = ({ count, testId }: MoreSeriesProps) =>
  count > 0 ? (
    <Typography
      variant="caption"
      data-testid={testId}
      sx={{ color: 'text.secondary', fontStyle: 'italic' }}
    >
      +{count} more series
    </Typography>
  ) : null;

const TrendsTooltip = ({
  active,
  payload,
  label,
  coordinate,
  viewBox,
  metricLabel,
  maxYScale,
  onHoveredSeriesChange,
}: TrendsTooltipProps) => {
  const date = active && typeof label === 'number' ? label : undefined;
  const rows = date !== undefined && payload ? toTooltipRows(payload) : [];

  // Recharts' shared tooltip reports every series in the hovered bucket, not
  // the line under the cursor, so the hovered line is taken to be the one
  // drawn closest to the cursor's height.
  const plotTop = viewBox?.y ?? 0;
  const plotHeight = viewBox?.height ?? 0;
  const cursorValue =
    coordinate?.y === undefined
      ? undefined
      : valueAtCursor(coordinate.y, plotTop, plotHeight, maxYScale);
  const hovered =
    cursorValue === undefined
      ? undefined
      : findNearestReading(rows, cursorValue);
  const hoveredName = hovered?.name ?? null;

  useEffect(() => {
    onHoveredSeriesChange?.(hoveredName);
  }, [hoveredName, onHoveredSeriesChange]);

  if (date === undefined || rows.length === 0) return null;

  const { shown, hiddenAbove, hiddenBelow } = selectTooltipRows(
    rows,
    hoveredName,
    MAX_TOOLTIP_ROWS,
  );

  return (
    <Paper
      elevation={3}
      data-testid="trends-chart-tooltip"
      sx={{
        p: 1,
        width: 260,
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
        {formatTooltipDate(date)}
      </Typography>

      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
        <MoreSeries count={hiddenAbove} testId="trends-tooltip-more-above" />
        {shown.map((row) => (
          <TooltipRowView
            key={row.name}
            row={row}
            metricLabel={metricLabel}
            isHovered={row.name === hoveredName}
          />
        ))}
        <MoreSeries count={hiddenBelow} testId="trends-tooltip-more-below" />
      </Box>
    </Paper>
  );
};

/**
 * Reads a series' value from a row. A function rather than a string path,
 * because a series name containing a dot would otherwise be read as a nested
 * lookup.
 */
const valueOf = (name: string) => (row: TrendsChartRow) =>
  row.values[name] ?? null;

/** The dot drawn on every reading of a line. */
const readingDot = (color: string) => ({
  r: 2.5,
  fill: color,
  stroke: colors.white,
  strokeWidth: 1.5,
});

interface TrendLinesProps {
  readonly rows: readonly TrendsChartRow[];
  readonly plotted: readonly PlottedSeries[];
  readonly xDomain: [number, number] | undefined;
  readonly xTicks: number[];
  readonly yTicks: number[];
  readonly maxYScale: number;
  readonly axisTickStyle: SVGProps<SVGTextElement>;
  readonly metricLabel: string;
  readonly syncId: string;
  readonly onHoveredSeriesChange: (name: string | null) => void;
}

/**
 * Every plotted line, with axes and tooltip. Memoized and independent of the
 * hovered series, so moving between lines does not redraw it.
 */
const TrendLines = memo(function TrendLines({
  rows,
  plotted,
  xDomain,
  xTicks,
  yTicks,
  maxYScale,
  axisTickStyle,
  metricLabel,
  syncId,
  onHoveredSeriesChange,
}: TrendLinesProps) {
  return (
    <ResponsiveContainer width="100%" height={CHART_HEIGHT}>
      <LineChart
        // Trust that the chart will not modify readonly data.
        data={rows as TrendsChartRow[]}
        margin={CHART_MARGIN}
        syncId={syncId}
        onMouseLeave={() => onHoveredSeriesChange(null)}
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
          height={TIME_AXIS_HEIGHT}
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
          width={VALUE_AXIS_WIDTH}
        />
        <Tooltip
          isAnimationActive={false}
          wrapperStyle={TOOLTIP_WRAPPER_STYLE}
          cursor={{
            stroke: colors.grey[500],
            strokeWidth: 1,
            strokeDasharray: '3 3',
          }}
          content={
            <TrendsTooltip
              metricLabel={metricLabel}
              maxYScale={maxYScale}
              onHoveredSeriesChange={onHoveredSeriesChange}
            />
          }
        />
        {plotted.map(({ name, color }) => (
          // Keyed by name, never by index: an index key lets
          // React reuse the node, so deselecting a series makes
          // its neighbour inherit the wrong data.
          <Line
            key={name}
            name={name}
            data-testid={`series-line-${name}`}
            dataKey={valueOf(name)}
            type="linear"
            stroke={color}
            strokeWidth={LINE_WIDTH}
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
            dot={readingDot(color)}
            activeDot={{ r: 4, strokeWidth: 1.5, stroke: colors.white }}
            // Background refetches would otherwise make the
            // chart redraw itself for no visible reason.
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
});

/** Renders nothing; lets the highlight layer track the hovered bucket. */
const NoTooltip = () => null;

interface HighlightLayerProps {
  readonly rows: readonly TrendsChartRow[];
  readonly series: PlottedSeries;
  readonly xDomain: [number, number] | undefined;
  readonly maxYScale: number;
  readonly syncId: string;
}

/**
 * The hovered line, drawn again in a transparent chart laid over the main
 * one. SVG has no z-index, so this is how the hovered line ends up on top
 * without reordering, and so re-rendering, the main chart's lines. It only
 * ever holds one line, so re-rendering it on hover is cheap.
 *
 * The axes are kept, invisible, so the plot area lines up with the main
 * chart's; a hidden axis would give its space back to the plot.
 */
const HighlightLayer = ({
  rows,
  series,
  xDomain,
  maxYScale,
  syncId,
}: HighlightLayerProps) => (
  <Box
    data-testid="trends-highlight-layer"
    sx={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}
  >
    <ResponsiveContainer width="100%" height={CHART_HEIGHT}>
      <LineChart
        data={rows as TrendsChartRow[]}
        margin={CHART_MARGIN}
        syncId={syncId}
      >
        <XAxis
          type="number"
          scale="time"
          dataKey="timestampMs"
          domain={xDomain}
          height={TIME_AXIS_HEIGHT}
          tick={false}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          domain={[0, maxYScale]}
          width={VALUE_AXIS_WIDTH}
          tick={false}
          tickLine={false}
          axisLine={false}
        />
        <Tooltip content={NoTooltip} cursor={false} isAnimationActive={false} />
        <Line
          name={series.name}
          data-testid={`series-highlight-${series.name}`}
          dataKey={valueOf(series.name)}
          type="linear"
          stroke={series.color}
          strokeWidth={HIGHLIGHTED_LINE_WIDTH}
          strokeLinecap="round"
          strokeLinejoin="round"
          connectNulls={false}
          dot={readingDot(series.color)}
          activeDot={{ r: 5, strokeWidth: 1.5, stroke: colors.white }}
          isAnimationActive={false}
        />
      </LineChart>
    </ResponsiveContainer>
  </Box>
);

export interface HistoricalAvailabilityTrendsChartProps {
  filter?: string;
}

export const HistoricalAvailabilityTrendsChart = ({
  filter = '',
}: HistoricalAvailabilityTrendsChartProps) => {
  const theme = useTheme();
  const [viewBy, setViewBy] = useState<TrendlineGrouping>(
    TrendlineGrouping.GROUP_BY_OVERALL,
  );
  const [colorSlots, setColorSlots] = useState<SeriesColorSlots>({});
  // The series whose line is under the cursor, as resolved by the tooltip.
  const [hoveredSeries, setHoveredSeries] = useState<string | null>(null);

  const queryRequest = useMemo(
    () => ({
      grouping: viewBy,
      startTime: undefined,
      endTime: undefined,
      filter,
    }),
    [viewBy, filter],
  );

  const { data, isLoading, isError, error } =
    useFleetAvailabilityTrends(queryRequest);

  const seriesList = useMemo(() => {
    const series = data?.series ?? [];
    if (viewBy === TrendlineGrouping.GROUP_BY_POOL) {
      return sortPoolSeries(series);
    }
    return series;
  }, [data?.series, viewBy]);

  // The default selection is seeded once per query key (grouping + filter), as soon as that
  // query's first response arrives. It deliberately does not re-run on background refetches
  // with unchanged query parameters to preserve manual user selections and color assignments.
  const seededQueryKey = useRef<string | null>(null);
  const currentQueryKey = `${viewBy}::${filter}`;

  useEffect(() => {
    // Overall is a single unselectable series, so it needs no slots.
    if (viewBy === TrendlineGrouping.GROUP_BY_OVERALL) {
      if (seededQueryKey.current !== currentQueryKey) {
        seededQueryKey.current = currentQueryKey;
        setColorSlots({});
      }
      return;
    }

    // Wait for the response before choosing defaults.
    if (isLoading) return;

    if (seededQueryKey.current !== currentQueryKey) {
      seededQueryKey.current = currentQueryKey;
      const count =
        viewBy === TrendlineGrouping.GROUP_BY_POOL
          ? DEFAULT_VISIBLE_SERIES_POOL
          : DEFAULT_VISIBLE_SERIES_MODEL;
      const initial: SeriesColorSlots = {};
      seriesList.slice(0, count).forEach((series, idx) => {
        initial[series.name] = idx;
      });
      setColorSlots(initial);
    }
  }, [viewBy, isLoading, seriesList, currentQueryKey]);

  const handleToggleSeries = (name: string) => {
    setColorSlots((prev) => {
      if (name in prev) {
        const next = { ...prev };
        delete next[name];
        return next;
      }

      const slot = findFreeColorSlot(prev);
      return { ...prev, [name]: slot };
    });
  };

  const handleSelectAll = () => {
    setColorSlots((prev) => {
      const next = { ...prev };
      seriesList.forEach((series) => {
        if (!(series.name in next)) {
          next[series.name] = findFreeColorSlot(next);
        }
      });
      return next;
    });
  };

  const activeCount = useMemo(
    () => seriesList.filter((series) => series.name in colorSlots).length,
    [seriesList, colorSlots],
  );

  const isAllSelected =
    seriesList.length > 0 && activeCount === seriesList.length;
  const isIndeterminate = activeCount > 0 && activeCount < seriesList.length;

  const handleToggleMaster = () => {
    if (isAllSelected) {
      setColorSlots({});
    } else {
      handleSelectAll();
    }
  };

  const activeSeries = useMemo(() => {
    if (viewBy === TrendlineGrouping.GROUP_BY_OVERALL) {
      return seriesList;
    }
    return seriesList.filter((series) => series.name in colorSlots);
  }, [viewBy, seriesList, colorSlots]);

  const rows = useMemo(
    () => buildTrendsChartRows(activeSeries),
    [activeSeries],
  );

  const plotted: PlottedSeries[] = useMemo(
    () =>
      activeSeries.map((series) => ({
        name: series.name,
        // Overall has no selector and therefore no slot, so it falls back to
        // slot 0.
        color: getSeriesColor(colorSlots[series.name] ?? 0),
      })),
    [activeSeries, colorSlots],
  );

  // Only emphasize a line when there are others to tell it apart from, and
  // only while it is still plotted: deselecting the hovered series, or
  // switching grouping, must not leave every remaining line faded.
  const emphasizedSeries =
    plotted.length > 1 &&
    hoveredSeries !== null &&
    plotted.some((series) => series.name === hoveredSeries)
      ? hoveredSeries
      : null;

  const highlighted = useMemo(
    () => plotted.find((series) => series.name === emphasizedSeries),
    [plotted, emphasizedSeries],
  );

  // Links the main chart to the highlight layer, so the layer's active dot
  // follows the hovered bucket. Unique per chart instance.
  const syncId = useId();

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

  const isAvailability = data?.metricType === TrendlineMetricType.AVAILABILITY;

  const metricLabel = isAvailability ? 'Availability' : 'Health';

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
                <BaselineChip
                  isAvailability={isAvailability}
                  testId="trends-baseline-chip"
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
      <CardContent sx={{ p: 2, pb: 1, '&:last-child': { pb: 1 } }}>
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
                  display: 'flex',
                  flexDirection: 'column',
                  height: CHART_HEIGHT,
                  minWidth: 0,
                }}
              >
                {/* Fixed Header with Tristate Checkbox (never scrolls away) */}
                <Box sx={{ pb: 0.5, flexShrink: 0 }}>
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                    }}
                  >
                    <FormControlLabel
                      control={
                        <Checkbox
                          size="small"
                          checked={isAllSelected}
                          indeterminate={isIndeterminate}
                          onChange={handleToggleMaster}
                          slotProps={{
                            input: {
                              'aria-label': 'Select all series',
                            },
                          }}
                        />
                      }
                      label={
                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                          Select all
                        </Typography>
                      }
                      sx={{ mr: 0 }}
                    />
                    <Typography
                      variant="caption"
                      sx={{
                        color: 'text.secondary',
                        bgcolor: 'action.hover',
                        px: 0.75,
                        py: 0.2,
                        borderRadius: 1,
                        fontSize: 11,
                        fontWeight: 600,
                      }}
                    >
                      {activeCount} of {seriesList.length}
                    </Typography>
                  </Box>
                </Box>

                <Divider sx={{ my: 0.75, flexShrink: 0 }} />

                {/* Scrollable Checkbox List */}
                <Box
                  sx={{
                    flexGrow: 1,
                    overflowY: 'auto',
                    overflowX: 'hidden',
                    pr: 1,
                    minWidth: 0,
                    '&::-webkit-scrollbar': { width: '4px' },
                    '&::-webkit-scrollbar-thumb': {
                      backgroundColor: 'divider',
                      borderRadius: '4px',
                    },
                  }}
                >
                  <FormGroup sx={{ width: '100%', minWidth: 0 }}>
                    {seriesList.map((series) => {
                      const slot = colorSlots[series.name];
                      const isChecked = slot !== undefined;
                      const color = isChecked
                        ? getSeriesColor(slot)
                        : undefined;
                      return (
                        <FormControlLabel
                          key={series.name}
                          sx={{
                            width: '100%',
                            minWidth: 0,
                            mr: 0,
                            '& .MuiFormControlLabel-label': {
                              minWidth: 0,
                              width: '100%',
                              overflow: 'hidden',
                            },
                          }}
                          control={
                            <Checkbox
                              size="small"
                              checked={isChecked}
                              onChange={() => handleToggleSeries(series.name)}
                              sx={
                                isChecked
                                  ? {
                                      color,
                                      '&.Mui-checked': {
                                        color,
                                      },
                                    }
                                  : undefined
                              }
                            />
                          }
                          label={
                            <Typography
                              variant="body2"
                              title={series.name}
                              sx={{
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                                display: 'block',
                              }}
                            >
                              {series.name}
                            </Typography>
                          }
                        />
                      );
                    })}
                  </FormGroup>
                </Box>
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
                  data-highlighted-series={highlighted?.name}
                  role="img"
                  aria-label="Fleet historical availability and health trendlines chart"
                  sx={{
                    width: '100%',
                    position: 'relative',
                    // Fading is done in CSS rather than through line props, so
                    // hovering never re-renders the main chart: with ~100
                    // series that costs a full recharts layout per change.
                    ...(highlighted && {
                      '& .trends-main-chart .recharts-line': {
                        opacity: FADED_OPACITY,
                      },
                      // The highlight layer draws the only active dot.
                      '& .trends-main-chart .recharts-active-dot': {
                        display: 'none',
                      },
                    }),
                  }}
                >
                  <Box className="trends-main-chart">
                    <TrendLines
                      rows={rows}
                      plotted={plotted}
                      xDomain={xDomain}
                      xTicks={xTicks}
                      yTicks={yTicks}
                      maxYScale={maxYScale}
                      axisTickStyle={axisTickStyle}
                      metricLabel={metricLabel}
                      syncId={syncId}
                      onHoveredSeriesChange={setHoveredSeries}
                    />
                  </Box>
                  {highlighted && (
                    <HighlightLayer
                      rows={rows}
                      series={highlighted}
                      xDomain={xDomain}
                      maxYScale={maxYScale}
                      syncId={syncId}
                    />
                  )}
                </Box>
              )}
            </Box>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};
