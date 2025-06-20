// src/test_investigation/components/test_info/overview/history_rate_display_section.tsx
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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { Box, Link, Typography, Tooltip } from '@mui/material';
import { DateTime } from 'luxon';
import { memo, useMemo } from 'react';

import {
  SemanticStatusType,
  getStatusStyle,
} from '@/common/styles/status_styles';
import { displayApproxDuartion } from '@/common/tools/time_utils/time_utils';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { NO_HISTORY_DATA_TEXT } from '@/test_investigation/components/test_info/constants';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { constructTestHistoryUrl } from '@/test_investigation/utils/test_info_utils';

import { useTestVariantBranch } from '../context/context';

interface FailureRateProps {
  segment: Segment;
  segmentContextType: 'invocation' | 'beforeInvocation' | 'afterInvocation';
  isMostRecentSegment?: boolean; // True if this segment is the newest of all segments
  nowDtForFormatting?: DateTime;
}
const FailureRate = memo(function FailureRateView({
  segment,
  segmentContextType,
  nowDtForFormatting,
  isMostRecentSegment,
}: FailureRateProps) {
  const formattedRate = getFormattedFailureRateFromSegment(segment);
  const style = getStatusStyle(getFailureRateStatusTypeFromSegment(segment));
  const IconComponent = style.icon;

  const startHourDisplay = formatSegmentTimestampForDisplay(
    segment.startHour,
    nowDtForFormatting,
  );

  let descriptiveText: string;
  if (segmentContextType === 'invocation') {
    if (isMostRecentSegment) {
      descriptiveText = `${formattedRate} now failing`;
    } else {
      descriptiveText = `${formattedRate} failing at invocation`;
    }
  } else if (segmentContextType === 'beforeInvocation') {
    descriptiveText = `${formattedRate} failed`;
  } else {
    descriptiveText = `${formattedRate} failing`;
  }

  return (
    <Tooltip
      title={
        `Segment: ${Number(segment.startPosition)} - ${Number(segment.endPosition)}` +
        (startHourDisplay ? ` (started ${startHourDisplay})` : '') +
        (segmentContextType === 'invocation'
          ? ' (Invocation Commit Segment)'
          : '')
      }
      arrow
    >
      <Box
        sx={{
          padding: '2px 8px 2px 4px',
          borderRadius: '4px',
          backgroundColor: style.backgroundColor || 'transparent',
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
        }}
      >
        {IconComponent && (
          <IconComponent
            sx={{
              fontSize: '1.1rem',
              color: style.iconColor || style.textColor,
            }}
          />
        )}
        <Typography component="span" variant="caption">
          {descriptiveText}
        </Typography>
      </Box>
    </Tooltip>
  );
});

interface SegmentArrowProps {
  pointingToSegment: Segment;
  nowDtForFormatting: DateTime;
  project: string | undefined;
  testId: string;
  variantHash: string;
  refHash: string;
}

const SegmentArrow = memo(function ArrowDisplayWrapper({
  pointingToSegment,
  nowDtForFormatting,
  project,
  testId,
  variantHash,
  refHash,
}: SegmentArrowProps) {
  const blamelistBaseUrl = `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`;
  const blamelistLink = `${blamelistBaseUrl}#CP-${pointingToSegment.startPosition}`;

  const durationText =
    formatSegmentTimestampForDisplay(
      pointingToSegment.startHour,
      nowDtForFormatting,
    ) || '';
  const durationDisplay = durationText.trim() ? (
    <Typography
      variant="caption"
      color="text.secondary"
      sx={{ lineHeight: 'normal', textAlign: 'center', whiteSpace: 'nowrap' }}
    >
      {durationText}
    </Typography>
  ) : (
    <Box sx={{ height: 'calc(1em * 1.2)' }} /> // Approx height of a caption line to prevent collapse
  );

  return (
    <Box
      sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}
    >
      {durationDisplay}
      <ArrowBackIcon
        data-testid="ArrowBackIcon"
        sx={{ color: 'text.disabled', my: 0.25, fontSize: '20px' }}
      />
      {project && ( // if project was not supplied, the link will be invalid.
        <Link
          href={blamelistLink}
          target="_blank"
          rel="noopener noreferrer"
          variant="caption"
          sx={{ lineHeight: 'normal', whiteSpace: 'nowrap' }}
        >
          blamelist
        </Link>
      )}
    </Box>
  );
});

interface TestAddedInfoProps {
  segment: Segment;
  nowDtForFormatting: DateTime;
  project: string | undefined;
  testId: string;
  variantHash: string;
  refHash: string;
}

const TestAddedInfo = memo(function TestAddedDisplay({
  segment,
  nowDtForFormatting,
  project,
  testId,
  variantHash,
  refHash,
}: TestAddedInfoProps) {
  const startTimeAgo = formatSegmentTimestampForDisplay(
    segment.startHour,
    nowDtForFormatting,
  );
  const blamelistBaseUrl = `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`;
  const blamelistLink = `${blamelistBaseUrl}#CP-${segment.startPosition}`;

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        textAlign: 'center',
        p: 1,
      }}
    >
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ lineHeight: 'normal', whiteSpace: 'nowrap' }}
      >
        Test added {startTimeAgo}
      </Typography>
      {project && ( // if project was not supplied, the link will be invalid.
        <Link
          href={blamelistLink}
          target="_blank"
          rel="noopener noreferrer"
          variant="caption"
          sx={{ lineHeight: 'normal', whiteSpace: 'nowrap', mt: 0.25 }}
        >
          blamelist
        </Link>
      )}
    </Box>
  );
});

function formatSegmentTimestampForDisplay(
  isoString?: string,
  nowDateTime: DateTime = DateTime.now(),
): string | undefined {
  if (!isoString) return undefined;
  const pastDateTime = DateTime.fromISO(isoString, { zone: 'utc' });
  if (!pastDateTime.isValid) return undefined;
  const approxDurationText = displayApproxDuartion(
    nowDateTime.diff(pastDateTime),
  );
  return approxDurationText && approxDurationText !== 'N/A'
    ? `${approxDurationText} ago`
    : undefined;
}

function determineRateStatusType(ratePercent: number): SemanticStatusType {
  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}

function getFormattedFailureRateFromSegment(segment: Segment): string {
  if (!segment || !segment.counts) {
    return 'N/A';
  }
  const { unexpectedResults = 0, totalResults = 0 } = segment.counts;
  const ratePercent =
    totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;
  return `${ratePercent.toFixed(0)}%`;
}

function getFailureRateStatusTypeFromSegment(
  segment: Segment,
): SemanticStatusType {
  if (!segment || !segment.counts) {
    return 'unknown';
  }
  const { unexpectedResults = 0, totalResults = 0 } = segment.counts;
  const ratePercent =
    totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;
  return determineRateStatusType(ratePercent);
}

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

  return (
    <Box>
      <Typography variant="body2" color="textSecondary" sx={{ mb: 1 }}>
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
            {/* Leftmost item: Newer segment if the invocation segment is not the most recent. */}
            {invocationSegmentIndex > 0 && (
              <>
                <FailureRate
                  segment={segments[invocationSegmentIndex - 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  segmentContextType="afterInvocation"
                />

                <SegmentArrow
                  pointingToSegment={segments[invocationSegmentIndex - 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  project={project}
                  testId={testVariant.testId}
                  variantHash={testVariant.variantHash}
                  refHash={testVariantBranch.refHash}
                />
              </>
            )}

            {/* Invocation Segment */}
            <FailureRate
              segment={segments[invocationSegmentIndex]}
              segmentContextType="invocation"
              nowDtForFormatting={nowDtForFormatting}
              isMostRecentSegment={invocationSegmentIndex === 0}
            />

            {invocationSegmentIndex < segments.length - 1 ? (
              <>
                {/* If an older segment than the one containing the invocation exists */}
                <SegmentArrow
                  key="arrow-invocation-to-older"
                  pointingToSegment={segments[invocationSegmentIndex]}
                  nowDtForFormatting={nowDtForFormatting}
                  project={project}
                  testId={testVariant.testId}
                  variantHash={testVariant.variantHash}
                  refHash={testVariantBranch.refHash}
                />
                <FailureRate
                  segment={segments[invocationSegmentIndex + 1]}
                  nowDtForFormatting={nowDtForFormatting}
                  segmentContextType="beforeInvocation"
                />
              </>
            ) : (
              <TestAddedInfo
                segment={segments[invocationSegmentIndex]}
                nowDtForFormatting={nowDtForFormatting}
                project={project}
                testId={testVariant.testId}
                variantHash={testVariant.variantHash}
                refHash={testVariantBranch.refHash}
              />
            )}
          </Box>
          {allTestHistoryLink && (
            <Box>
              <Link
                href={allTestHistoryLink}
                variant="caption"
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
