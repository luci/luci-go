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

import { getStatusStyle } from '@/common/styles/status_styles';
import {
  Segment,
  Segment_Counts,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import {
  formatSegmentTimestamp,
  getFailureRateStatusTypeFromSegmentCount,
  getFormattedFailureRateFromSegmentCount,
} from './util';

interface CommitRangeProps {
  readonly segment: Segment;
  readonly nowDtForFormatting?: DateTime;
  readonly blamelistBaseUrl?: string;
}

function CommitRange({
  segment,
  nowDtForFormatting,
  blamelistBaseUrl,
}: CommitRangeProps) {
  const startHourDisplay = formatSegmentTimestamp(
    segment.startHour,
    nowDtForFormatting,
  );
  const endHourDisplay = formatSegmentTimestamp(
    segment.endHour,
    nowDtForFormatting,
  );

  const createBlamelistLink = (position: string) => {
    return blamelistBaseUrl
      ? `${blamelistBaseUrl}?expand=${`CP-${position}`}#CP-${position}`
      : undefined;
  };

  return (
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        borderBottom: 1,
        borderColor: 'divider',
        pb: 1,
        mb: 1,
      }}
    >
      <Box>
        <Typography variant="caption" display="block">
          End:{' '}
          {blamelistBaseUrl ? (
            <Link
              href={createBlamelistLink(segment.endPosition)}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {Number(segment.endPosition)}
            </Link>
          ) : (
            Number(segment.endPosition)
          )}
        </Typography>
        {segment.endHour && (
          <Typography variant="caption" display="block" color="text.secondary">
            ({endHourDisplay})
          </Typography>
        )}
      </Box>

      <Typography variant="caption" color="text.secondary" sx={{ mx: 1 }}>
        {Number(segment.endPosition) - Number(segment.startPosition) + 1}{' '}
        commits
      </Typography>

      <Box sx={{ textAlign: 'right' }}>
        <Typography variant="caption" display="block">
          Start:{' '}
          {blamelistBaseUrl ? (
            <Link
              href={createBlamelistLink(segment.startPosition)}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {Number(segment.startPosition)}
            </Link>
          ) : (
            Number(segment.startPosition)
          )}
        </Typography>
        {segment.startHour && (
          <Typography variant="caption" display="block" color="text.secondary">
            ({startHourDisplay})
          </Typography>
        )}
      </Box>
    </Box>
  );
}

interface ResultCountsBreakdownProps {
  readonly counts: Segment_Counts;
}

function ResultCountsBreakdown({ counts }: ResultCountsBreakdownProps) {
  const formattedRate = getFormattedFailureRateFromSegmentCount(counts);
  const statusType = getFailureRateStatusTypeFromSegmentCount(counts);
  const style = getStatusStyle(statusType);
  const IconComponent = style.icon;

  const unexpectedResults = counts.unexpectedResults ?? 0;
  const totalResults = counts.totalResults ?? 0;
  const failPercent =
    totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;

  const passedStyle = getStatusStyle('passed');
  const failedStyle = getStatusStyle('failed');

  const background =
    totalResults > 0
      ? `linear-gradient(to right, ${failedStyle.backgroundColor} 0%, ` +
        `${failedStyle.backgroundColor} ${failPercent}%, ` +
        `${passedStyle.backgroundColor} ${failPercent}%, ` +
        `${passedStyle.backgroundColor} 100%)`
      : style.backgroundColor;

  return (
    <Box sx={{ textAlign: 'center', mb: 1 }}>
      <Typography variant="caption" color="text.secondary">
        Before Retries
      </Typography>
      <Box
        sx={{
          borderRadius: 1,
          background,
          border: 1,
          borderColor: 'divider',
          mt: 0.25,
          p: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 0.5,
        }}
      >
        {IconComponent && (
          <IconComponent
            sx={{
              fontSize: '18px',
              color: style.iconColor || style.textColor,
            }}
          />
        )}
        <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
          {`${formattedRate} of ${totalResults} test results failed`}
        </Typography>
      </Box>
    </Box>
  );
}

interface VerdictCountsBreakdownProps {
  readonly counts: Segment_Counts;
}

function formatPercentage(count: number, total: number) {
  if (total === 0) return '0%';
  return (count / total).toLocaleString(undefined, {
    style: 'percent',
    maximumFractionDigits: 1,
    minimumFractionDigits: 0,
  });
}

function VerdictCountsBreakdown({ counts }: VerdictCountsBreakdownProps) {
  const unexpectedCount = counts.unexpectedVerdicts || 0;
  const flakyCount = counts.flakyVerdicts || 0;
  const total = counts.totalVerdicts || 0;
  const expectedCount = Math.max(0, total - unexpectedCount - flakyCount);

  const failedStyle = getStatusStyle('failed');
  const flakyStyle = getStatusStyle('flaky');
  const passedStyle = getStatusStyle('passed');

  const failPercent = total > 0 ? (unexpectedCount / total) * 100 : 0;
  const flakyPercent = total > 0 ? (flakyCount / total) * 100 : 0;
  const failPlusFlakyPercent = failPercent + flakyPercent;

  const background =
    total > 0
      ? `linear-gradient(to right, ` +
        `${failedStyle.backgroundColor} 0%, ` +
        `${failedStyle.backgroundColor} ${failPercent}%, ` +
        `${flakyStyle.backgroundColor} ${failPercent}%, ` +
        `${flakyStyle.backgroundColor} ${failPlusFlakyPercent}%, ` +
        `${passedStyle.backgroundColor} ${failPlusFlakyPercent}%, ` +
        `${passedStyle.backgroundColor} 100%)`
      : undefined;

  return (
    <Box sx={{ textAlign: 'center' }}>
      <Typography variant="caption" color="text.secondary">
        After Retries
      </Typography>
      <Box
        sx={{
          borderRadius: 1,
          background,
          border: 1,
          borderColor: 'divider',
          mt: 0.25,
          p: 1,
        }}
      >
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
          }}
        >
          <Box sx={{ opacity: unexpectedCount ? 1 : 0.6 }}>
            <Typography variant="caption">failed</Typography>
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              {formatPercentage(unexpectedCount, total)}
            </Typography>
          </Box>
          <Box sx={{ opacity: flakyCount ? 1 : 0.6 }}>
            <Typography variant="caption">flaky</Typography>
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              {formatPercentage(flakyCount, total)}
            </Typography>
          </Box>
          <Box sx={{ opacity: expectedCount ? 1 : 0.6 }}>
            <Typography variant="caption">passed</Typography>
            <Typography variant="body2" sx={{ fontWeight: 'bold' }}>
              {formatPercentage(expectedCount, total)}
            </Typography>
          </Box>
        </Box>
        <Typography variant="caption">of {total} test verdicts</Typography>
      </Box>
    </Box>
  );
}

export interface SegmentTooltipProps {
  segment: Segment;
  segmentContextType?: 'invocation' | 'beforeInvocation' | 'afterInvocation';
  nowDtForFormatting?: DateTime;
  blamelistBaseUrl?: string;
}

/**
 * An informative tooltip giving the user more details about a segment in the history display.
 */
export function SegmentTooltip({
  segment,
  segmentContextType,
  nowDtForFormatting,
  blamelistBaseUrl,
}: SegmentTooltipProps) {
  return (
    <Box sx={{ p: 1 }}>
      <CommitRange
        segment={segment}
        nowDtForFormatting={nowDtForFormatting}
        blamelistBaseUrl={blamelistBaseUrl}
      />

      {segment.counts && (
        <>
          <ResultCountsBreakdown counts={segment.counts} />
          <VerdictCountsBreakdown counts={segment.counts} />
        </>
      )}

      {segmentContextType === 'invocation' && (
        <Typography variant="subtitle2" sx={{ mt: 1, textAlign: 'center' }}>
          This segment contains the current test result
        </Typography>
      )}
      {segmentContextType === 'afterInvocation' && (
        <Typography variant="subtitle2" sx={{ mt: 1, textAlign: 'center' }}>
          This segment is newer than the current test result
        </Typography>
      )}
      {segmentContextType === 'beforeInvocation' && (
        <Typography variant="subtitle2" sx={{ mt: 1, textAlign: 'center' }}>
          This segment is older than the current test result
        </Typography>
      )}
    </Box>
  );
}
