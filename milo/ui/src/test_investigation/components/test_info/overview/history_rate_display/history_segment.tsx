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
import { memo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  getStatusStyle,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { formatSegmentTimestamp } from './util';

interface HistorySegmentTooltip {
  segment: Segment;
  segmentContextType: 'invocation' | 'beforeInvocation' | 'afterInvocation';
  nowDtForFormatting?: DateTime;
  blamelistBaseUrl: string | undefined;
}

/**
 * An informative tooltip giving the user more details about a segment in the history display.
 */
function HistorySegmentTooltip({
  segment,
  segmentContextType,
  nowDtForFormatting,
  blamelistBaseUrl,
}: HistorySegmentTooltip) {
  const formattedRate = getFormattedFailureRateFromSegment(segment);
  const startHourDisplay = formatSegmentTimestamp(
    segment.startHour,
    nowDtForFormatting,
  );
  const endHourDisplay = formatSegmentTimestamp(
    segment.endHour,
    nowDtForFormatting,
  );

  const style = getStatusStyle(getFailureRateStatusTypeFromSegment(segment));

  const createBlamelistLink = (position: string) => {
    return `${blamelistBaseUrl}?expand=${`CP-${position}`}#CP-${position}`;
  };

  return (
    <Box sx={{ p: 1 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          width: '100%',
          mb: 3,
        }}
      >
        <Box
          sx={{
            width: '100%',
            borderRadius: '4px',
            backgroundColor: style.backgroundColor,
            border: `1px solid ${style.borderColor}`,
            my: 1,
            p: 1,
            textAlign: 'center',
          }}
        >
          <Typography variant="body2">
            Failure Rate: {formattedRate}
            {segment.counts &&
              ` (${segment.counts.unexpectedResults} / ${segment.counts.totalResults} failed)`}
          </Typography>
        </Box>

        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            width: '100%',
          }}
        >
          <Box sx={{ textAlign: 'left' }}>
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
              <Typography
                variant="caption"
                display="block"
                color="text.secondary"
              >
                ({endHourDisplay})
              </Typography>
            )}
          </Box>
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
              <Typography
                variant="caption"
                display="block"
                color="text.secondary"
              >
                ({startHourDisplay})
              </Typography>
            )}
          </Box>
        </Box>
      </Box>

      {segmentContextType === 'invocation' && (
        <Typography variant="subtitle2" gutterBottom>
          This segment contains the current test result
        </Typography>
      )}
      {segmentContextType === 'afterInvocation' && (
        <Typography variant="subtitle2" gutterBottom>
          This segment is newer than the current test result
        </Typography>
      )}
      {segmentContextType === 'beforeInvocation' && (
        <Typography variant="subtitle2" gutterBottom>
          This segment is older than the current test result
        </Typography>
      )}
      {segment.counts && (
        <Typography variant="caption" color="text.secondary" component="div">
          Contains {segment.counts.totalRuns} verdicts across{' '}
          {segment.counts.totalVerdicts} distinct commits.
        </Typography>
      )}
    </Box>
  );
}

interface HistorySegmentProps {
  segment: Segment;
  segmentContextType: 'invocation' | 'beforeInvocation' | 'afterInvocation';
  isMostRecentSegment?: boolean; // True if this segment is the newest of all segments
  nowDtForFormatting?: DateTime;
  blamelistBaseUrl: string | undefined;
}

/**
 * Displays a single segment of the test history, showing its failure rate and
 * providing a tooltip with more details.
 */
export const HistorySegment = memo(function HistorySegment({
  segment,
  segmentContextType,
  nowDtForFormatting,
  isMostRecentSegment,
  blamelistBaseUrl,
}: HistorySegmentProps) {
  const formattedRate = getFormattedFailureRateFromSegment(segment);
  const style = getStatusStyle(getFailureRateStatusTypeFromSegment(segment));
  const IconComponent = style.icon;

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
    <HtmlTooltip
      title={
        <HistorySegmentTooltip
          segment={segment}
          segmentContextType={segmentContextType}
          nowDtForFormatting={nowDtForFormatting}
          blamelistBaseUrl={blamelistBaseUrl}
        />
      }
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
    </HtmlTooltip>
  );
});

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

function determineRateStatusType(ratePercent: number): SemanticStatusType {
  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}
