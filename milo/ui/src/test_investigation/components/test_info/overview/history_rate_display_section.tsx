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

import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
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
import { NO_HISTORY_DATA_TEXT } from '@/test_investigation/components/test_info/types';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import { constructTestHistoryUrl } from '@/test_investigation/utils/test_info_utils';

import { useTestVariantBranch } from '../context/context';

interface FailureRateInfo {
  formattedRate: string;
  statusType: SemanticStatusType;
}

interface FailureRateViewProps {
  segment: Segment;
  isContextual?: boolean;
  nowDtForFormatting?: DateTime;
}
const FailureRateView = memo(function FailureRateView({
  segment,
  isContextual,
  nowDtForFormatting,
}: FailureRateViewProps) {
  const rateInfo = createFailureRateInfoFromSegment(segment);
  const style = getStatusStyle(rateInfo.statusType);
  const startHourDisplay = formatSegmentTimestampForDisplay(
    segment.startHour,
    nowDtForFormatting,
  );
  return (
    <Tooltip
      title={
        `Segment: ${Number(segment.startPosition)} - ${Number(segment.endPosition)}` +
        (startHourDisplay ? ` (started ${startHourDisplay})` : '') +
        (isContextual ? ' (Contextual to Invocation Commit)' : '')
      }
      arrow
    >
      <Box
        sx={{
          border: isContextual
            ? '2px solid var(--gm3-color-primary)'
            : '1px solid transparent',
          padding: isContextual ? '2px' : '3px',
          borderRadius: '4px',
          borderColor: isContextual
            ? 'var(--gm3-color-primary)'
            : style.borderColor,
          backgroundColor: style.backgroundColor || 'transparent',
          flexGrow:
            parseInt(segment.endPosition) - parseInt(segment.startPosition),
          display: 'flex',
          justifyContent: 'center',
        }}
      >
        <Typography
          component="span"
          variant="body2"
          sx={{
            color: style.textColor,
            fontWeight: 'medium',
            padding: '2px 4px',
            borderRadius: '4px',
            textAlign: 'center',
          }}
        >
          {rateInfo.formattedRate}
        </Typography>
      </Box>
    </Tooltip>
  );
});

// --- Visual indicators ---
const VerticalBar = memo(function VerticalBar({
  tooltipTitle,
}: {
  tooltipTitle: string;
}) {
  return (
    <Tooltip title={tooltipTitle} arrow>
      <Box
        sx={{
          width: '3px',
          height: '20px',
          backgroundColor: 'text.disabled',
          alignSelf: 'center',
          borderRadius: '1px',
        }}
      />
    </Tooltip>
  );
});

const EllipsisIndicator = memo(function EllipsisIndicator({
  tooltipTitle,
}: {
  tooltipTitle: string;
}) {
  return (
    <Tooltip title={tooltipTitle} arrow>
      <Typography
        variant="h6"
        sx={{
          color: 'text.disabled',
          lineHeight: 'normal',
          px: 0.5,
          alignSelf: 'center',
        }}
      >
        ...
      </Typography>
    </Tooltip>
  );
});

function isoStringToLuxonDateTime(isoString?: string): DateTime | undefined {
  if (!isoString) return undefined;
  const dt = DateTime.fromISO(isoString, { zone: 'utc' });
  return dt.isValid ? dt : undefined;
}

function formatSegmentTimestampForDisplay(
  isoString?: string,
  nowDateTime: DateTime = DateTime.now(),
): string | undefined {
  if (!isoString) return undefined;
  const pastDateTime = isoStringToLuxonDateTime(isoString);
  if (!pastDateTime || !pastDateTime.isValid) return undefined;
  const duration = nowDateTime.diff(pastDateTime);
  const approxDurationText = displayApproxDuartion(duration);
  return approxDurationText && approxDurationText !== 'N/A'
    ? `${approxDurationText} ago`
    : undefined;
}

function determineRateStatusType(ratePercent: number): SemanticStatusType {
  if (ratePercent <= 5) return 'success';
  if (ratePercent > 5 && ratePercent < 95) return 'warning';
  if (ratePercent >= 95) return 'error';
  return 'unknown';
}

function createFailureRateInfoFromSegment(segment: Segment): FailureRateInfo {
  if (!segment.counts) {
    return { formattedRate: 'N/A', statusType: 'unknown' };
  }
  const { unexpectedResults = 0, totalResults = 0 } = segment.counts;
  const ratePercent =
    totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;
  const statusType = determineRateStatusType(ratePercent);
  return {
    formattedRate: `${ratePercent.toFixed(0)}%`,
    statusType,
  };
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

  if (!segments || segments.length === 0) {
    return (
      <Typography variant="body2" color="text.disabled" sx={{ mb: 1 }}>
        {NO_HISTORY_DATA_TEXT}
      </Typography>
    );
  }
  return (
    <Box>
      <Typography variant="body2" gutterBottom color="textSecondary">
        Postsubmit history (Changepoint failure rate)
      </Typography>

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
          {segments.length <= invocationSegmentIndex + 2 ? (
            <VerticalBar
              key="end-bar"
              tooltipTitle="This is the oldest recorded history"
            />
          ) : (
            <>
              <EllipsisIndicator
                key="end-ellipsis"
                tooltipTitle="Older history available"
              />
              <ArrowForwardIcon
                sx={{ color: 'text.disabled', alignSelf: 'center' }}
              />
            </>
          )}
          {segments.length > invocationSegmentIndex + 1 && (
            <>
              <FailureRateView
                segment={segments[invocationSegmentIndex + 1]}
                nowDtForFormatting={nowDtForFormatting}
              />
              <ArrowForwardIcon
                sx={{ color: 'text.disabled', alignSelf: 'center' }}
              />
            </>
          )}

          <FailureRateView
            segment={segments[invocationSegmentIndex]}
            isContextual
            nowDtForFormatting={nowDtForFormatting}
          />
          {invocationSegmentIndex > 0 && (
            <>
              <ArrowForwardIcon
                sx={{ color: 'text.disabled', alignSelf: 'center' }}
              />
              <FailureRateView
                segment={segments[invocationSegmentIndex - 1]}
                nowDtForFormatting={nowDtForFormatting}
              />
            </>
          )}
          {invocationSegmentIndex < 2 ? (
            <VerticalBar
              key="start-bar"
              tooltipTitle="This is the newest recorded history"
            />
          ) : (
            <>
              <ArrowForwardIcon
                sx={{ color: 'text.disabled', alignSelf: 'center' }}
              />
              <EllipsisIndicator
                key="start-ellipsis"
                tooltipTitle="More recent history available"
              />
            </>
          )}
        </Box>
        {allTestHistoryLink && (
          <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
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
              View full history
            </Link>
          </Box>
        )}
      </Box>
    </Box>
  );
}
