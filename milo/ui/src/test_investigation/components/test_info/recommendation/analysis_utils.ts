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

import { SemanticStatusType } from '@/common/styles/status_styles';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { isPresubmitRun } from '@/test_investigation/utils/test_info_utils';

export interface AnalysisItemContent {
  status?: SemanticStatusType;
  text: string;
}

export interface AnalysisData {
  invocation: Invocation;
  testVariant: TestVariant;
  segments: readonly Segment[] | undefined;
  /** Current time to use in any analyses requiring it (e.g. for calculating human readable times like "3 hours ago") */
  now: DateTime;
}

// Type for individual analysis point generating functions
export type AnalysisPointGenerator = (
  data: AnalysisData,
) => AnalysisItemContent[];

// Helper functions.
/**
 * Finds the segment corresponding to the invocation's commit position.
 * Returns the segment's index in the segments array.
 */
export function findInvocationSegment(
  segments: readonly Segment[],
  invocation: Invocation | null,
): number {
  const position = parseInt(
    invocation?.sourceSpec?.sources?.gitilesCommit?.position || '',
    10,
  );
  if (isNaN(position)) {
    return -1;
  }

  for (let i = 0; i < (segments || []).length; i++) {
    const start = Number(segments[i].startPosition);
    const end = Number(segments[i].endPosition);
    if (position >= start && position <= end) {
      return i;
    }
  }
  return -1;
}

export function segmentFailureRatePercent(segment: Segment) {
  const counts = segment.counts;
  if (!counts || !counts?.totalResults) {
    return undefined;
  }
  return Math.round(
    ((counts.unexpectedResults || 0) / counts.totalResults) * 100,
  );
}

// --- Individual Analysis Point Generators ---
export const noSources: AnalysisPointGenerator = ({ invocation }) => {
  if (!invocation.sourceSpec?.sources?.gitilesCommit?.position) {
    return [
      {
        text: 'History analysis is unavailable because no source commit position information was uploaded with this invocation.',
        status: 'warning',
      },
    ];
  }
  return [];
};

export const explainFlaky: AnalysisPointGenerator = ({ testVariant }) => {
  if (testVariant.statusV2 === TestVerdict_Status.FLAKY) {
    return [
      {
        text: 'This test is flaky (contained both pass and fail results).  It is counted as a pass.',
        status: 'success',
      },
    ];
  }
  return [];
};

export const explainExonerated: AnalysisPointGenerator = ({ testVariant }) => {
  if (testVariant.statusOverride === TestVerdict_StatusOverride.EXONERATED) {
    return [
      {
        text: 'This test is exonerated.  Any failures have been explicitly ignored and this result is counted as a pass.',
        status: 'success',
      },
    ];
  }
  return [];
};

export const invocationNotInSegment: AnalysisPointGenerator = (data) => {
  const { invocation, testVariant, segments } = data;

  // Preconditions
  if (
    !isPresubmitRun(invocation) ||
    testVariant.statusV2 !== TestVerdict_Status.FAILED ||
    !segments ||
    segments.length === 0
  ) {
    return [];
  }

  const invocationSegment = findInvocationSegment(segments, invocation);
  if (invocationSegment === -1) {
    return [
      {
        text: `The source postion of this invocation does not belong to any test result segment
                so no history analysis is possible.  Please report this as a bug.`,
        status: 'warning',
      },
    ];
  }
  return [];
};

export const presubmitBrokenInPostsubmit: AnalysisPointGenerator = (data) => {
  const { invocation, testVariant, segments } = data;
  const points: AnalysisItemContent[] = [];

  // Preconditions
  if (
    !isPresubmitRun(invocation) ||
    testVariant.statusV2 !== TestVerdict_Status.FAILED ||
    !segments ||
    segments.length === 0
  ) {
    return [];
  }

  const invocationSegment = findInvocationSegment(segments, invocation);
  if (invocationSegment === -1) {
    return [];
  }

  const invocationSegmentFailureRate = segmentFailureRatePercent(
    segments[invocationSegment],
  );

  const latestSegmentFailureRate = segmentFailureRatePercent(segments[0]);

  if (
    invocationSegmentFailureRate === undefined ||
    latestSegmentFailureRate === undefined
  ) {
    return [];
  }

  if (invocationSegmentFailureRate < 95) {
    return [];
  }

  points.push({
    text: `This test was broken in postsubmit at the commit that this presubmit was tested on.`,
    status: 'error',
  });

  if (latestSegmentFailureRate <= 5) {
    points.push({
      text: `This test is now passing in postsubmit. It is likely to pass if you rerun.`,
      status: 'success',
    });
  } else if (latestSegmentFailureRate >= 95) {
    points.push({
      text: `This test is still failing in postsubmit. It is likely to fail again if you rerun.`,
      status: 'error',
    });
  } else {
    points.push({
      text: `This test is still flaky in postsubmit (~${latestSegmentFailureRate}% failure rate). Rerunning may fail again.`,
      status: 'warning',
    });
  }
  return points;
};

export const presubmitFlakyInPostsubmit: AnalysisPointGenerator = (data) => {
  const { invocation, segments } = data;
  const points: AnalysisItemContent[] = [];

  // Preconditions
  if (!isPresubmitRun(invocation) || !segments || segments.length === 0) {
    return [];
  }

  const invocationSegment = findInvocationSegment(segments, invocation);
  if (invocationSegment === -1) {
    return [];
  }
  const invocationSegmentFailureRate = segmentFailureRatePercent(
    segments[invocationSegment],
  );

  const latestSegmentFailureRate = segmentFailureRatePercent(segments[0]);

  if (
    invocationSegmentFailureRate === undefined ||
    latestSegmentFailureRate === undefined
  ) {
    return [];
  }

  if (invocationSegmentFailureRate <= 5 || invocationSegmentFailureRate >= 95) {
    return [];
  }

  points.push({
    text: `This test was flaky in postsubmit (~${invocationSegmentFailureRate}% failure rate) at the
          commit that this presubmit was tested on.`,
    status: 'warning',
  });

  if (latestSegmentFailureRate <= 5) {
    points.push({
      text: `This test is now passing in postsubmit. It is likely to pass if you rerun.`,
      status: 'success',
    });
  } else if (latestSegmentFailureRate >= 95) {
    points.push({
      text: `This test is now failing in postsubmit. It is likely to fail if you rerun.`,
      status: 'warning',
    });
  } else {
    points.push({
      text: `This test is still flaky in postsubmit (~${latestSegmentFailureRate}% failure rate). Rerunning may fail again.`,
      status: 'warning',
    });
  }
  return points;
};

export const postsubmitCurrentStatus: AnalysisPointGenerator = (data) => {
  const { invocation, segments } = data;
  const points: AnalysisItemContent[] = [];

  // Preconditions
  if (isPresubmitRun(invocation) || !segments || segments.length === 0) {
    return [];
  }

  const invocationSegment = findInvocationSegment(segments, invocation);
  if (invocationSegment === -1) {
    return [];
  }

  const invocationSegmentFailureRate = segmentFailureRatePercent(
    segments[invocationSegment],
  );
  if (invocationSegmentFailureRate === undefined) {
    return [];
  }

  let alreadyFixed = false;
  if (invocationSegmentFailureRate > 5) {
    for (let i = invocationSegment; i >= 0; i--) {
      const failureRate = segmentFailureRatePercent(segments[i]);
      if (failureRate === undefined) continue;
      if (failureRate <= 5) {
        const time = segments[i].startHour
          ? displayApproxDuration(
              data.now.diff(DateTime.fromISO(segments[i].startHour!)),
            )
          : '';
        points.push({
          text: `This test was already fixed ${time} ago`,
          status: 'success',
        });
        alreadyFixed = true;
        break;
      }
    }
  }

  if (!alreadyFixed) {
    const latestSegmentFailureRate = segmentFailureRatePercent(segments[0]);
    if (latestSegmentFailureRate !== undefined) {
      const nowFailing = latestSegmentFailureRate >= 95;
      const nowFlaky =
        latestSegmentFailureRate > 5 && latestSegmentFailureRate < 95;
      const wasFailing = invocationSegmentFailureRate >= 95;
      const wasFlaky =
        invocationSegmentFailureRate > 5 && invocationSegmentFailureRate < 95;

      if (wasFailing && nowFailing) {
        points.push({
          text: `This test is still failing`,
          status: 'error',
        });
      } else if (wasFailing && nowFlaky) {
        points.push({
          text: `This test is now flaky (~${latestSegmentFailureRate}% failure rate)`,
          status: 'error',
        });
      } else if (wasFlaky && nowFlaky) {
        points.push({
          text: `This test is still flaky (~${latestSegmentFailureRate}% failure rate)`,
          status: 'success',
        });
      } else if (wasFlaky && nowFailing) {
        {
          points.push({
            text: `This test is now failing consistently (~${latestSegmentFailureRate}% failure rate)`,
            status: 'success',
          });
        }
      }
    }
  }

  return points;
};

export function generateAnalysisPoints(
  currentTimeForAgoDt: DateTime,
  rawSegments: readonly Segment[] | undefined,
  invocation: Invocation,
  testVariant: TestVariant,
): AnalysisItemContent[] {
  const analysisGenerators: AnalysisPointGenerator[] = [
    noSources,
    explainFlaky,
    explainExonerated,
    invocationNotInSegment,
    presubmitBrokenInPostsubmit,
    presubmitFlakyInPostsubmit,
    postsubmitCurrentStatus,
  ];

  return analysisGenerators.flatMap((generator) => {
    return generator({
      invocation,
      testVariant,
      segments: rawSegments,
      now: currentTimeForAgoDt,
    });
  });
}
