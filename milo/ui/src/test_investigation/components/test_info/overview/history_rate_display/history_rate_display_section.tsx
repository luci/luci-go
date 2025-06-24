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

import { Box, Link, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { useMemo } from 'react';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { NO_HISTORY_DATA_TEXT } from '@/test_investigation/components/test_info/constants';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { constructTestHistoryUrl } from '@/test_investigation/utils/test_info_utils';

import { useTestVariantBranch } from '../../context';

import { HistoryChangepoint } from './history_changepoint';
import { HistorySegment } from './history_segment';
import { TestAddedInfo } from './test_added_info';

function findInvocationSegmentIndex(
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
  if (segments.length > 0 && position > Number(segments[0].startPosition)) {
    // Result is newer than segment analysis, assume latest segment continues to include this result.
    return 0;
  }
  return -1;
}

interface HistoryRateDisplaySectionProps {
  currentTimeForAgo?: Date;
}

/**
 * Displays the history rate of a test variant, including segments and changepoints.
 * It determines which segments to display based on the current invocation's position.
 */
export function HistoryRateDisplaySection({
  currentTimeForAgo: currentTimeForAgoProp,
}: HistoryRateDisplaySectionProps) {
  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();
  const segments = testVariantBranch?.segments;

  const allTestHistoryLink = useMemo(
    () =>
      constructTestHistoryUrl(
        project,
        testVariant.testId,
        testVariant.variant?.def,
      ),
    [project, testVariant?.testId, testVariant?.variant],
  );

  const nowDtForFormatting = useMemo(
    () => DateTime.fromJSDate(currentTimeForAgoProp || new Date()),
    [currentTimeForAgoProp],
  );

  const invocationSegmentIndex = useMemo(
    () => findInvocationSegmentIndex(segments || [], invocation),
    [segments, invocation],
  );

  const testId = testVariant.testId;
  const variantHash = testVariant.variantHash;
  const refHash = testVariantBranch?.refHash;
  const blamelistBaseUrl = refHash
    ? `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`
    : undefined;

  return (
    <Box>
      <Typography variant="body2" color="textSecondary">
        Postsubmit history
      </Typography>

      {(!segments || segments.length === 0) && (
        <Typography variant="body2" color="text.disabled" sx={{ mb: 1 }}>
          {NO_HISTORY_DATA_TEXT}
        </Typography>
      )}

      {segments && segments.length > 0 && invocationSegmentIndex === -1 && (
        <Typography variant="body2" color="text.disabled" sx={{ mb: 1 }}>
          No history segments matching the source position of this verdict can
          be found.
        </Typography>
      )}

      {segments && segments.length > 0 && invocationSegmentIndex !== -1 && (
        <Box sx={{ mb: 1 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              mb: 0.5,
              flexWrap: 'wrap',
            }}
          >
            {/* Leftmost segment: Present if the invocation segment is not the most recent. */}
            {invocationSegmentIndex > 0 && (
              <>
                <HistorySegment
                  segment={segments[invocationSegmentIndex - 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  segmentContextType="afterInvocation"
                  blamelistBaseUrl={blamelistBaseUrl}
                />

                <HistoryChangepoint
                  pointingToSegment={segments[invocationSegmentIndex - 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  blamelistBaseUrl={blamelistBaseUrl}
                />
              </>
            )}

            {/* Invocation segment: Always present */}
            <HistorySegment
              segment={segments[invocationSegmentIndex]}
              segmentContextType="invocation"
              nowDtForFormatting={nowDtForFormatting}
              isMostRecentSegment={invocationSegmentIndex === 0}
              blamelistBaseUrl={blamelistBaseUrl}
            />

            {/*
            Rightmost segment: An older segment than the one containing the invocation if it exists,
            else an explicit display that this test was added
            */}
            {invocationSegmentIndex < segments.length - 1 ? (
              <>
                <HistoryChangepoint
                  key="arrow-invocation-to-older"
                  pointingToSegment={segments[invocationSegmentIndex]}
                  nowDtForFormatting={nowDtForFormatting}
                  blamelistBaseUrl={blamelistBaseUrl}
                />
                <HistorySegment
                  segment={segments[invocationSegmentIndex + 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  segmentContextType="beforeInvocation"
                  blamelistBaseUrl={blamelistBaseUrl}
                />
              </>
            ) : (
              <TestAddedInfo
                segment={segments[invocationSegmentIndex]}
                nowDtForFormatting={nowDtForFormatting}
                blamelistBaseUrl={blamelistBaseUrl}
              />
            )}
          </Box>
          {allTestHistoryLink && (
            <Box>
              <Link
                href={allTestHistoryLink}
                variant="caption"
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  ml: 'auto',
                  whiteSpace: 'nowrap',
                  alignSelf: 'center',
                  pl: 1,
                }}
              >
                View full postsubmit history
              </Link>
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
}
