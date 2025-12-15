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

import { DateTime } from 'luxon';

import { SegmentSummary } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';
import { OutputTestVerdict } from '@/common/types/verdict';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

// Constants
export const THIRTY_DAYS_MS = 1000 * 60 * 60 * 24 * 30;
export const SIX_MONTHS_MS = 6 * THIRTY_DAYS_MS;

export function findInvocationSegmentIndex(
  summaries: SegmentSummary[],
  currentBuildId: string,
): number {
  if (!summaries || !currentBuildId) return -1;

  for (let i = 0; i < summaries.length; i++) {
    const s = summaries[i];
    const startId = s.start_result?.build_id;
    const endId = s.end_result?.build_id;

    if (!startId || !endId) continue;

    // Assuming segments are sorted descending (Newest -> Oldest)
    // startId is the "newest" build in the segment, endId is the "oldest"
    // So range is [endId, startId]
    // We use localeCompare with numeric: true to handle alphanumeric build IDs (e.g. P123) correctly.
    // This correctly sorts "P2" < "P10", and handles standard numeric IDs as well.
    const inRange =
      currentBuildId.localeCompare(endId, undefined, { numeric: true }) >= 0 &&
      currentBuildId.localeCompare(startId, undefined, { numeric: true }) <= 0;

    if (inRange) {
      return i;
    }
  }
  return -1;
}

export function calculateTargetTimestamp(invocation: AnyInvocation): number {
  const createTime = invocation.createTime
    ? DateTime.fromISO(invocation.createTime).toMillis()
    : Date.now();
  return createTime - SIX_MONTHS_MS;
}

// Minimal interface for what we need from build data
interface BuildData {
  builds?: {
    buildId: string;
    creationTimestamp?: string;
  }[];
}

export function calculateStartBuildId(
  buildBeforeData: BuildData | undefined,
  buildAfterData: BuildData | undefined,
  targetTimestampMs: number,
): string | undefined {
  const buildBefore = buildBeforeData?.builds?.[0];
  const buildAfter = buildAfterData?.builds?.[0];

  if (!buildBefore && !buildAfter) return undefined;
  if (!buildBefore) return buildAfter?.buildId;
  if (!buildAfter) return buildBefore?.buildId;

  const timeBefore = Number(buildBefore.creationTimestamp || 0);
  const timeAfter = Number(buildAfter.creationTimestamp || 0);
  const diffBefore = Math.abs(targetTimestampMs - timeBefore);
  const diffAfter = Math.abs(targetTimestampMs - timeAfter);

  return diffBefore <= diffAfter ? buildBefore.buildId : buildAfter.buildId;
}

export function getOrSynthesizeSummaries(
  summariesData: { summaries?: { summaries?: SegmentSummary[] }[] } | undefined,
  currentBuildId: string | undefined,
  testVariant: OutputTestVerdict,
): SegmentSummary[] {
  const loadedSummaries = summariesData?.summaries?.[0]?.summaries || [];

  if (loadedSummaries.length > 0) {
    return loadedSummaries;
  }

  // If no history found, synthetic segment for current invocation
  if (currentBuildId && testVariant) {
    // Crude mapping from status -> approximate rate
    // EXPECTED -> 0%
    // UNEXPECTED -> 100%
    // FLAKY -> 50%
    let rate = 0;
    let failures = '0';
    let total = '1';

    const status = testVariant.statusV2 || testVariant.status;

    if (
      status === TestVerdict_Status.FAILED ||
      status === TestVerdict_Status.EXECUTION_ERRORED
    ) {
      rate = 1;
      failures = '1';
    } else if (status === TestVerdict_Status.FLAKY) {
      rate = 0.5;
      failures = '1';
      total = '2';
    }

    const syntheticSegment: SegmentSummary = {
      start_result: {
        test_result_id: '',
        build_id: currentBuildId,
        invocation_id: '',
      },
      end_result: {
        test_result_id: '',
        build_id: currentBuildId,
        invocation_id: '',
      },
      clusters: [],
      health: {
        fail_rate: {
          rate,
          failures,
          total,
        },
      },
    };
    return [syntheticSegment];
  }

  return [];
}

export interface VisibleSegmentsResult {
  visibleSegments: {
    segment: SegmentSummary;
    type: 'afterInvocation' | 'invocation' | 'beforeInvocation';
  }[];
  ellipsisNewer: boolean;
  ellipsisOlder: boolean;
}

export function calculateVisibleSegments(
  summaries: SegmentSummary[],
  currentBuildId: string,
): VisibleSegmentsResult {
  const currentIndex = findInvocationSegmentIndex(
    summaries,
    currentBuildId || '',
  );
  if (currentIndex === -1)
    return {
      visibleSegments: [],
      ellipsisNewer: false,
      ellipsisOlder: false,
    };

  // Compress to: [Prev (Newer), Current, Next (Older)]
  // Segments are sorted Descending (Newest -> Oldest)
  // index - 1 is Newer
  // index + 1 is Older
  const segments: {
    segment: SegmentSummary;
    type: 'afterInvocation' | 'invocation' | 'beforeInvocation';
  }[] = [];

  // Add Newer (if exists)
  if (currentIndex > 0) {
    segments.push({
      segment: summaries[currentIndex - 1],
      type: 'afterInvocation',
    });
  }

  // Add Current
  segments.push({
    segment: summaries[currentIndex],
    type: 'invocation',
  });

  // Add Older (if exists)
  if (currentIndex < summaries.length - 1) {
    segments.push({
      segment: summaries[currentIndex + 1],
      type: 'beforeInvocation',
    });
  }

  return {
    visibleSegments: segments,
    // If we skipped segments before currentIndex - 1
    ellipsisNewer: currentIndex > 1,
    // If we skipped segments after currentIndex + 1
    ellipsisOlder: currentIndex < summaries.length - 2,
  };
}
