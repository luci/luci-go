// Copyright 2025 The LUCI Authors.
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

import { Box, CircularProgress, Typography } from '@mui/material';
import { useMemo } from 'react';

import { useListBuilds } from '@/common/hooks/gapi_query/android_build/android_build';
import { SortingType } from '@/common/hooks/gapi_query/android_build/types';
import { useGetTestResultFluxgateSegmentSummaries } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

import { AndroidHistoryChangepoint } from './android_history_changepoint';
import { AndroidHistorySegment } from './android_history_segment';
import { getAntsTestIdentifierHash } from './hashing_utils';
import {
  THIRTY_DAYS_MS,
  calculateStartBuildId,
  calculateTargetTimestamp,
  calculateVisibleSegments,
  getOrSynthesizeSummaries,
} from './util';

export function AndroidTestTimeline() {
  const invocation = useInvocation();
  const testVariant = useTestVariant();

  // Ensure we are working with a RootInvocation and have primary build info
  const rootInvocation = isRootInvocation(invocation) ? invocation : null;
  const androidBuild = rootInvocation?.primaryBuild?.androidBuild;

  const branch = androidBuild?.branch;
  const target = androidBuild?.buildTarget;
  // Use source build ID as the anchor for current build.
  const currentBuildId =
    rootInvocation?.sources?.submittedAndroidBuild?.buildId;

  // Calculate timestamp for 6 months ago (search window for start)
  const targetTimestampMs = useMemo(
    () => calculateTargetTimestamp(invocation),
    [invocation],
  );

  const targetTimestampStr = useMemo(
    () => new Date(targetTimestampMs).toISOString(),
    [targetTimestampMs],
  );

  // Calculate timestamps for 30 day windows around target
  const olderWindowTimestampMs = useMemo(
    () => targetTimestampMs - THIRTY_DAYS_MS,
    [targetTimestampMs],
  );
  const newerWindowTimestampMs = useMemo(
    () => targetTimestampMs + THIRTY_DAYS_MS,
    [targetTimestampMs],
  );

  const olderWindowTimestampStr = useMemo(
    () => new Date(olderWindowTimestampMs).toISOString(),
    [olderWindowTimestampMs],
  );
  const newerWindowTimestampStr = useMemo(
    () => new Date(newerWindowTimestampMs).toISOString(),
    [newerWindowTimestampMs],
  );

  const antsTestId = useMemo(
    () =>
      rootInvocation
        ? getAntsTestIdentifierHash(rootInvocation, testVariant)
        : null,
    [rootInvocation, testVariant],
  );

  // 1. Fetch Latest Build (to serve as ending_build_id for range)
  const { data: latestBuildData, isLoading: isLoadingLatest } = useListBuilds(
    {
      branches: branch ? [branch] : undefined,
      targets: target ? [target] : undefined,
      page_size: 1,
      sorting_type: SortingType.BUILD_ID, // Latest first
    },
    {
      enabled: !!branch && !!target && !!antsTestId && !!currentBuildId,
      staleTime: 5 * 60 * 1000,
    },
  );

  // 2. Fetch ~6 Months Ago Builds (to find starting_build_id)

  // builds older than target: range [target, target-30d]
  // start (recent) = target
  // end (older) = target-30d
  // sort DESC (Newest First) -> Closest to target
  const { data: buildBeforeData, isLoading: isLoadingBefore } = useListBuilds(
    {
      branches: [branch!],
      targets: [target!],
      start_creation_timestamp: targetTimestampStr,
      end_creation_timestamp: olderWindowTimestampStr,
      page_size: 1,
      sorting_type: SortingType.BUILD_ID,
    },
    {
      enabled: !!branch && !!target && !!antsTestId && !!currentBuildId,
      staleTime: Infinity,
    },
  );

  // builds newer than target: range [target+30d, target]
  // start (recent) = target+30d
  // end (older) = target
  // sort ASC (Oldest First) -> Closest to target
  const { data: buildAfterData, isLoading: isLoadingAfter } = useListBuilds(
    {
      branches: [branch!],
      targets: [target!],
      start_creation_timestamp: newerWindowTimestampStr,
      end_creation_timestamp: targetTimestampStr,
      page_size: 1,
      sorting_type: SortingType.BUILD_ID_ASC,
    },
    {
      enabled: !!branch && !!target && !!antsTestId && !!currentBuildId,
      staleTime: Infinity,
    },
  );

  // Determine the best start build ID (closest to 6 months ago)
  const startBuildId = useMemo(
    () =>
      calculateStartBuildId(buildBeforeData, buildAfterData, targetTimestampMs),
    [buildBeforeData, buildAfterData, targetTimestampMs],
  );

  // Determine end build ID (latest available)
  const endBuildId = latestBuildData?.builds?.[0]?.buildId || currentBuildId;

  const { data: summariesData, isLoading: isLoadingSummaries } =
    useGetTestResultFluxgateSegmentSummaries(
      {
        test_identifier_ids: antsTestId ? [antsTestId] : [],
        range: {
          starting_build_id: startBuildId,
          ending_build_id: endBuildId,
        },
        combination_strategy: {
          combine_overlapping: true,
        },
      },
      {
        enabled:
          !!startBuildId && !!endBuildId && !!antsTestId && !!currentBuildId,
        staleTime: 5 * 60 * 1000,
      },
    );

  const summaries = useMemo(
    () => getOrSynthesizeSummaries(summariesData, currentBuildId, testVariant),
    [summariesData, currentBuildId, testVariant],
  );

  const { visibleSegments, ellipsisNewer, ellipsisOlder } = useMemo(
    () => calculateVisibleSegments(summaries, currentBuildId || ''),
    [summaries, currentBuildId],
  );

  if (!currentBuildId) {
    return null;
  }

  const isFetchingHistory =
    !!branch &&
    !!target &&
    (isLoadingBefore || isLoadingAfter || isLoadingLatest);

  const isFetchingSummaries =
    !!startBuildId && !!endBuildId && isLoadingSummaries;

  if (isFetchingHistory || isFetchingSummaries) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
        <CircularProgress size={24} />
        <Typography sx={{ ml: 1 }}>Loading timeline...</Typography>
      </Box>
    );
  }

  if (!summaries.length) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography variant="body2" color="text.secondary">
          No timeline data available.
        </Typography>
      </Box>
    );
  }

  if (!visibleSegments.length) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography variant="body2" color="text.secondary">
          Current invocation not found in history segments.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ overflowX: 'auto', pb: 2 }}>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Test timeline
      </Typography>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        {/* Newer Segments Ellipsis */}
        {ellipsisNewer && (
          <Typography variant="body2" color="text.secondary" sx={{ mx: 1 }}>
            ...
          </Typography>
        )}

        {visibleSegments.map((item, index) => {
          const isLast = index === visibleSegments.length - 1;

          // If this is the current invocation segment, we want to ensure any changepoint
          // link pointing *to* this segment uses the specific submittedBuildId of the
          // invocation, rather than the segment's endResult buildId (which might differ
          // if the segment covers a range).
          const segment =
            item.type === 'invocation' && item.segment.endResult
              ? {
                  ...item.segment,
                  endResult: {
                    ...item.segment.endResult,
                    buildId: currentBuildId || item.segment.endResult.buildId,
                  },
                }
              : item.segment;

          return (
            <Box
              key={index}
              sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
            >
              <AndroidHistorySegment
                segment={segment}
                segmentContextType={item.type}
                isMostRecentSegment={index === 0 && !ellipsisNewer}
              />
              {!isLast && (
                <AndroidHistoryChangepoint
                  pointingToSegment={segment}
                  olderSegment={visibleSegments[index + 1].segment}
                />
              )}
            </Box>
          );
        })}

        {/* Older Segments Ellipsis */}
        {ellipsisOlder && (
          <Typography variant="body2" color="text.secondary" sx={{ mx: 1 }}>
            ...
          </Typography>
        )}
      </Box>
    </Box>
  );
}
