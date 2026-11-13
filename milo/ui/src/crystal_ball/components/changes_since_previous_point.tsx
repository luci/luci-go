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

import { OpenInNew as OpenInNewIcon } from '@mui/icons-material';
import { Box, CircularProgress, Typography } from '@mui/material';
import { useMemo } from 'react';

import { Column, COMMON_MESSAGES } from '@/crystal_ball/constants';
import { useFetchWidgetRawSamples } from '@/crystal_ball/hooks';
import {
  buildAbClsSinceUrl,
  ChartPoint,
  createRawSamplesRequest,
  isSequentialBuildId,
  pointToSelectionContext,
} from '@/crystal_ball/utils';
import {
  PerfChartWidget,
  PerfDataSpec,
  PerfFilter,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ExternalCaptionLink } from './external_caption_link';

/**
 * Columns needed to identify the build of a raw sample.
 */
const BUILD_COLUMNS: readonly string[] = [
  Column.BUILD_BRANCH,
  Column.BUILD_ID,
  Column.BUILD_TARGET,
];

/**
 * Orders raw samples so that the newest build comes first.
 */
const NEWEST_BUILD_FIRST = `${Column.BUILD_CREATION_TIMESTAMP} desc`;

/**
 * Returns the message to show instead of the CL range link, or `undefined`
 * if the link can be shown.
 */
function getStatusMessage({
  hasRange,
  isError,
  isLoading,
  isSequential,
}: {
  hasRange: boolean;
  isError: boolean;
  isLoading: boolean;
  isSequential: boolean;
}): COMMON_MESSAGES | undefined {
  if (isError) return COMMON_MESSAGES.FAILED_TO_FETCH_CL_RANGE;
  if (isLoading) return COMMON_MESSAGES.LOOKING_UP_CHANGES;
  if (!isSequential) return COMMON_MESSAGES.CL_RANGE_UNAVAILABLE;
  if (!hasRange) return COMMON_MESSAGES.NO_PREVIOUS_BUILD_FOUND;
  return undefined;
}

/**
 * Props for the ChangesSincePreviousPoint component.
 */
export interface ChangesSincePreviousPointProps {
  dashboardName: string;
  dataSpecs?: { [key: string]: PerfDataSpec };
  globalFilters?: readonly PerfFilter[];
  /** The selected data point. */
  point?: ChartPoint;
  /** The data point preceding the selected point in the same series. */
  previousPoint: ChartPoint;
  seriesId?: string;
  seriesIndex?: number;
  widget: PerfChartWidget;
  widgetId: string;
}

/**
 * Displays a link to the list of CLs landed between the newest build of the
 * previous data point and the newest build of the selected data point.
 *
 * Works regardless of the chart's x-axis grouping: the newest build of each
 * point is looked up via its raw samples. The previous build is restricted to
 * the same branch and target as the selected build so the range is meaningful.
 */
export function ChangesSincePreviousPoint({
  dashboardName,
  dataSpecs,
  globalFilters,
  point,
  previousPoint,
  seriesId,
  seriesIndex,
  widget,
  widgetId,
}: ChangesSincePreviousPointProps) {
  const latestBuildRequest = useMemo(
    () =>
      createRawSamplesRequest({
        dashboardName,
        dataSpecs,
        extraColumns: BUILD_COLUMNS,
        globalFilters,
        orderBy: NEWEST_BUILD_FIRST,
        pageSize: 1,
        selectionContext: pointToSelectionContext(point),
        seriesId,
        seriesIndex,
        widget,
        widgetId,
      }),
    [
      dashboardName,
      dataSpecs,
      globalFilters,
      point,
      seriesId,
      seriesIndex,
      widget,
      widgetId,
    ],
  );
  const latestBuildQuery = useFetchWidgetRawSamples(latestBuildRequest);
  const latestRow = latestBuildQuery.data?.rows?.[0];
  const buildId = String(latestRow?.values?.[Column.BUILD_ID] ?? '');
  const buildBranch = String(latestRow?.values?.[Column.BUILD_BRANCH] ?? '');
  const buildTarget = String(latestRow?.values?.[Column.BUILD_TARGET] ?? '');
  const isSequential = isSequentialBuildId(buildId);

  const previousBuildRequest = useMemo(() => {
    const selectionContext = pointToSelectionContext(previousPoint);
    if (buildBranch) {
      selectionContext[Column.BUILD_BRANCH] = buildBranch;
    }
    if (buildTarget) {
      selectionContext[Column.BUILD_TARGET] = buildTarget;
    }
    return createRawSamplesRequest({
      dashboardName,
      dataSpecs,
      extraColumns: BUILD_COLUMNS,
      globalFilters,
      orderBy: NEWEST_BUILD_FIRST,
      pageSize: 1,
      selectionContext,
      seriesId,
      seriesIndex,
      widget,
      widgetId,
    });
  }, [
    buildBranch,
    buildTarget,
    dashboardName,
    dataSpecs,
    globalFilters,
    previousPoint,
    seriesId,
    seriesIndex,
    widget,
    widgetId,
  ]);
  const previousBuildQuery = useFetchWidgetRawSamples(previousBuildRequest, {
    enabled: isSequential,
  });
  const previousBuildId = String(
    previousBuildQuery.data?.rows?.[0]?.values?.[Column.BUILD_ID] ?? '',
  );

  const isError = latestBuildQuery.isError || previousBuildQuery.isError;
  const isLoading =
    !isError &&
    (latestBuildQuery.isLoading ||
      (isSequential && previousBuildQuery.isLoading));
  if (!isError && !isLoading && !latestRow) {
    // No samples for the selected point; nothing to link to.
    return null;
  }
  const hasRange =
    isSequentialBuildId(previousBuildId) &&
    Number(previousBuildId) < Number(buildId);
  const statusMessage = getStatusMessage({
    hasRange,
    isError,
    isLoading,
    isSequential,
  });

  return (
    <Box
      data-testid="changes-since-previous-point"
      sx={{
        px: 1.5,
        py: 0.75,
        borderBottom: '1px solid',
        borderColor: 'divider',
        display: 'flex',
        alignItems: 'baseline',
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
        {COMMON_MESSAGES.CHANGES}
      </Typography>
      {statusMessage ? (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {isLoading && <CircularProgress size={12} />}
          <Typography variant="caption" color="text.secondary">
            {statusMessage}
          </Typography>
        </Box>
      ) : (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25 }}>
          <ExternalCaptionLink
            href={buildAbClsSinceUrl(previousBuildId, buildId, buildTarget)}
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}
          >
            {COMMON_MESSAGES.VIEW_CLS_SINCE_PREVIOUS_POINT}
            <OpenInNewIcon sx={{ fontSize: 'inherit' }} />
          </ExternalCaptionLink>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}
          >
            {previousBuildId} → {buildId} | {buildBranch} | {buildTarget}
          </Typography>
        </Box>
      )}
    </Box>
  );
}
