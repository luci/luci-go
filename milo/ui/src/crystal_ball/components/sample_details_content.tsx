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
  ArrowDownward as ArrowDownwardIcon,
  ArrowUpward as ArrowUpwardIcon,
} from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';

import { RawSampleList, SelectedPointInfo } from '@/crystal_ball/components';
import {
  AGGREGATION_FUNCTION_LABELS,
  Column,
  COMMON_MESSAGES,
} from '@/crystal_ball/constants';
import { useFetchWidgetRawSamples } from '@/crystal_ball/hooks';
import { getSafeChartType } from '@/crystal_ball/utils';
import {
  FetchWidgetRawSamplesRequest,
  PerfChartSeries_PerfAggregationFunction,
  perfChartSeries_PerfAggregationFunctionFromJSON,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
  RawSampleRow,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Props for the SampleDetailsContent component.
 */
interface SampleDetailsContentProps {
  selectedPoint: SelectedPointInfo;
  widgetId: string;
  widget: PerfChartWidget;
  globalFilters?: readonly PerfFilter[];
  dataSpecs?: { [key: string]: PerfDataSpec };
  dashboardName: string;
}

/**
 * Component to display the raw samples for a selected point in a chart.
 * It fetches data using pagination and allows sorting.
 */
export function SampleDetailsContent({
  selectedPoint,
  widgetId,
  widget,
  globalFilters,
  dataSpecs,
  dashboardName,
}: SampleDetailsContentProps) {
  const pointObj = selectedPoint.point;
  const seriesId = selectedPoint.seriesId;
  const seriesIndex = selectedPoint.seriesIndex;

  const isDistribution = useMemo(
    () =>
      getSafeChartType(widget.chartType) ===
      PerfChartWidget_ChartType.INVOCATION_DISTRIBUTION,
    [widget.chartType],
  );

  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
  const [accumulatedRows, setAccumulatedRows] = useState<
    readonly RawSampleRow[]
  >([]);
  const [currentPageToken, setCurrentPageToken] = useState<string | undefined>(
    undefined,
  );
  const [expandedItems, setExpandedItems] = useState<Set<number>>(new Set());

  // Reset when dependencies change
  useEffect(() => {
    setAccumulatedRows([]);
    setCurrentPageToken(undefined);
    setExpandedItems(new Set());
  }, [
    sortDirection,
    selectedPoint.x,
    selectedPoint.y,
    selectedPoint.seriesName,
    widgetId,
  ]);

  const fetchRequest = useMemo(() => {
    const selectionContext: Record<string, string> = {};
    if (pointObj) {
      Object.entries(pointObj).forEach(([key, val]) => {
        if (key !== 'value' && key !== 'num_aggregated_rows') {
          selectionContext[key] = String(val);
        }
      });
    }
    if (isDistribution) {
      selectionContext['value'] = String(selectedPoint.y);
    }

    const orderBy = `value ${sortDirection}`;

    return FetchWidgetRawSamplesRequest.fromPartial({
      name: dashboardName,
      dashboardContent: PerfDashboardContent.fromPartial({
        globalFilters: globalFilters ?? [],
        dataSpecs: dataSpecs ?? {},
        widgets: [
          PerfWidget.fromPartial({
            id: widgetId,
            chart: PerfChartWidget.fromPartial({
              ...widget,
            }),
          }),
        ],
      }),
      widgetId,
      seriesId,
      seriesIndex:
        seriesIndex !== undefined && seriesIndex >= 0 ? seriesIndex : undefined,
      selectionContext,
      pageSize: 1000,
      orderBy,
      pageToken: currentPageToken,
      extraColumns: [
        Column.ATP_TEST_NAME,
        Column.BOARD,
        Column.BUILD_BRANCH,
        Column.BUILD_CREATION_TIMESTAMP,
        Column.BUILD_ID,
        Column.BUILD_TARGET,
        Column.BUILD_TYPE,
        Column.INVOCATION_COMPLETE_TIMESTAMP,
        Column.INVOCATION_URL,
        Column.METRIC_KEY,
        Column.MODEL,
        Column.PERFETTO_ARTIFACT_URL,
        Column.SKU,
        Column.TEST_NAME,
        Column.ANTS_INVOCATION_ID,
        Column.ANTS_TEST_RESULT_ID,
      ],
    });
  }, [
    globalFilters,
    dataSpecs,
    widgetId,
    widget,
    pointObj,
    dashboardName,
    seriesId,
    seriesIndex,
    selectedPoint,
    isDistribution,
    sortDirection,
    currentPageToken,
  ]);

  const { data, isLoading, isError, error, isFetching } =
    useFetchWidgetRawSamples(fetchRequest);

  useEffect(() => {
    if (data?.rows) {
      setAccumulatedRows((prev) => {
        const existingIds = new Set(
          prev.map((r) => String(r.values?.[Column.BUILD_ID] ?? '')),
        );
        const newRows = data.rows.filter(
          (r) => !existingIds.has(String(r.values?.[Column.BUILD_ID] ?? '')),
        );
        return [...prev, ...newRows];
      });
    }
  }, [data]);

  if (isLoading && accumulatedRows.length === 0) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (isError && accumulatedRows.length === 0) {
    return (
      <Alert severity="error" sx={{ my: 1 }}>
        {error?.message ?? COMMON_MESSAGES.FAILED_TO_FETCH_RAW_SAMPLES}
      </Alert>
    );
  }

  const seriesConfig =
    seriesId !== undefined
      ? widget.series?.find((s) => s.id === seriesId)
      : seriesIndex !== undefined && seriesIndex >= 0
        ? widget.series?.[seriesIndex]
        : undefined;
  const seriesColor = seriesConfig?.color;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <SeriesContextArea
        seriesColor={seriesColor}
        seriesName={selectedPoint.seriesName}
        value={selectedPoint.y}
        aggregation={seriesConfig?.aggregation}
        count={selectedPoint.count}
      />
      <ListControlsArea
        count={accumulatedRows.length}
        sortDirection={sortDirection}
        onSortToggle={() =>
          setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
        }
        hasMore={!!data?.nextPageToken}
        onExpandAll={() =>
          setExpandedItems(new Set(accumulatedRows.map((_, i) => i)))
        }
        onCollapseAll={() => setExpandedItems(new Set())}
      />
      <Box sx={{ flex: 1, overflowY: 'auto', p: 1.5, bgcolor: 'action.hover' }}>
        <RawSampleList
          rows={accumulatedRows}
          expandedItems={expandedItems}
          onToggleExpand={(index: number) => {
            setExpandedItems((prev) => {
              const next = new Set(prev);
              if (next.has(index)) {
                next.delete(index);
              } else {
                next.add(index);
              }
              return next;
            });
          }}
        />

        {data?.nextPageToken && (
          <Box sx={{ textAlign: 'center', py: 1 }}>
            <Button
              variant="outlined"
              size="small"
              onClick={() => setCurrentPageToken(data.nextPageToken)}
              disabled={isFetching}
            >
              {isFetching ? COMMON_MESSAGES.LOADING : COMMON_MESSAGES.LOAD_MORE}
            </Button>
          </Box>
        )}

        {isFetching && !data?.nextPageToken && (
          <Box sx={{ textAlign: 'center', py: 1 }}>
            <CircularProgress size={20} />
          </Box>
        )}

        {!data?.nextPageToken && !isFetching && (
          <Box sx={{ textAlign: 'center', py: 1 }}>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                textTransform: 'uppercase',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                letterSpacing: 0.5,
              }}
            >
              {COMMON_MESSAGES.END_OF_SAMPLES}
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  );
}

interface SeriesContextAreaProps {
  seriesColor?: string;
  seriesName?: string;
  value?: number;
  aggregation?: PerfChartSeries_PerfAggregationFunction;
  count?: number;
}

function SeriesContextArea({
  seriesColor,
  seriesName,
  value,
  aggregation,
  count,
}: SeriesContextAreaProps) {
  const numericAgg =
    typeof aggregation === 'string'
      ? perfChartSeries_PerfAggregationFunctionFromJSON(aggregation)
      : aggregation;

  const aggLabel =
    numericAgg !== undefined
      ? (
          AGGREGATION_FUNCTION_LABELS[numericAgg] ??
          PerfChartSeries_PerfAggregationFunction[numericAgg] ??
          'value'
        ).toLowerCase()
      : 'value';

  const unit = count === 1 ? 'sample' : 'samples';
  const verb = count === 1 ? 'has' : 'have';

  return (
    <Box
      sx={{
        px: 1.5,
        py: 0.5,
        borderBottom: '1px solid',
        borderColor: 'divider',
        display: 'flex',
        alignItems: 'center',
        gap: 1.5,
      }}
    >
      <Typography
        variant="caption"
        sx={{
          fontWeight: (theme) => theme.typography.fontWeightBold,
          color: 'text.secondary',
          textTransform: 'uppercase',
        }}
      >
        Series
      </Typography>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          px: 0.75,
          py: 0.15,
          bgcolor: 'action.hover',
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 10,
        }}
      >
        <Box
          sx={{
            width: 5,
            height: 5,
            borderRadius: '50%',
            bgcolor: seriesColor ?? 'text.primary',
          }}
        />
        <Typography
          variant="caption"
          sx={{
            fontFamily: 'monospace',
            color: 'text.primary',
          }}
        >
          {seriesName}
        </Typography>
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, ml: 'auto' }}>
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          <Typography
            component="span"
            variant="caption"
            sx={{ fontWeight: 'bold', color: 'text.primary' }}
          >
            {count?.toLocaleString()}
          </Typography>{' '}
          {unit} {verb} a{' '}
          <Typography
            component="span"
            variant="caption"
            sx={{ fontWeight: 'bold', color: 'text.primary' }}
          >
            {aggLabel}
          </Typography>{' '}
          of{' '}
          <Typography
            component="span"
            variant="caption"
            sx={{ fontWeight: 'bold', color: 'text.primary' }}
          >
            {value?.toLocaleString()}
          </Typography>
        </Typography>
      </Box>
    </Box>
  );
}

interface ListControlsAreaProps {
  count: number;
  sortDirection: 'asc' | 'desc';
  onSortToggle: () => void;
  hasMore?: boolean;
  onExpandAll: () => void;
  onCollapseAll: () => void;
}

function ListControlsArea({
  count,
  sortDirection,
  onSortToggle,
  hasMore,
  onExpandAll,
  onCollapseAll,
}: ListControlsAreaProps) {
  return (
    <Box
      sx={{
        px: 1.5,
        py: 0.25,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        bgcolor: 'action.hover',
        borderBottom: '1px solid',
        borderColor: 'divider',
      }}
    >
      <Typography variant="caption" color="text.secondary">
        {hasMore ? 'Showing' : COMMON_MESSAGES.FOUND}{' '}
        <strong>
          {count}
          {hasMore ? '+' : ''}
        </strong>{' '}
        {count !== 1 ? COMMON_MESSAGES.RAW_SAMPLES : COMMON_MESSAGES.RAW_SAMPLE}
      </Typography>

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <Button
          size="small"
          onClick={onExpandAll}
          variant="text"
          sx={{
            height: 22,
            fontSize: (theme) => theme.typography.caption.fontSize,
            fontWeight: 'bold',
            color: 'primary.main',
            minWidth: 0,
            p: 0,
            textTransform: 'none',
          }}
        >
          Expand All
        </Button>
        <Typography variant="caption" color="text.secondary" sx={{ mx: 0.5 }}>
          |
        </Typography>
        <Button
          size="small"
          onClick={onCollapseAll}
          variant="text"
          sx={{
            height: 22,
            fontSize: (theme) => theme.typography.caption.fontSize,
            fontWeight: 'bold',
            color: 'primary.main',
            minWidth: 0,
            p: 0,
            textTransform: 'none',
          }}
        >
          Collapse All
        </Button>
        <Button
          data-testid="sort-direction-btn"
          onClick={onSortToggle}
          size="small"
          variant="text"
          sx={{
            height: 22,
            fontSize: (theme) => theme.typography.caption.fontSize,
            px: 0.5,
            textTransform: 'none',
            display: 'flex',
            alignItems: 'center',
            gap: 0.25,
            color: 'text.secondary',
            fontWeight: 'bold',
            '&:hover': {
              bgcolor: 'action.hover',
            },
          }}
        >
          Value
          {sortDirection === 'asc' ? (
            <ArrowUpwardIcon
              sx={{
                fontSize: (theme) => theme.typography.caption.fontSize,
                color: 'text.secondary',
              }}
            />
          ) : (
            <ArrowDownwardIcon
              sx={{
                fontSize: (theme) => theme.typography.caption.fontSize,
                color: 'text.secondary',
              }}
            />
          )}
        </Button>
      </Box>
    </Box>
  );
}
