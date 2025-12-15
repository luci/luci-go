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

import { Box, Typography } from '@mui/material';
import { memo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { SegmentSummary } from '@/common/hooks/gapi_query/android_fluxgate/android_fluxgate';
import {
  getStatusStyle,
  SemanticStatusType,
} from '@/common/styles/status_styles';

interface AndroidHistorySegmentTooltipProps {
  segment: SegmentSummary;
  segmentContextType: 'invocation' | 'beforeInvocation' | 'afterInvocation';
}

function AndroidHistorySegmentTooltip({
  segment,
  segmentContextType,
}: AndroidHistorySegmentTooltipProps) {
  const formattedRate = getFormattedFailureRate(segment);
  const style = getStatusStyle(getFailureRateStatusType(segment));
  const startBuildId = segment.start_result?.build_id || 'Unknown';
  const endBuildId = segment.end_result?.build_id || 'Unknown';

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
            {segment.health?.fail_rate &&
              ` (${segment.health.fail_rate.failures} / ${segment.health.fail_rate.total} failed)`}
          </Typography>
        </Box>

        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            width: '100%',
            gap: 2,
          }}
        >
          <Box sx={{ textAlign: 'left' }}>
            <Typography variant="caption" display="block">
              End Build: {endBuildId}
            </Typography>
          </Box>
          <Box sx={{ textAlign: 'right' }}>
            <Typography variant="caption" display="block">
              Start Build: {startBuildId}
            </Typography>
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
    </Box>
  );
}

interface AndroidHistorySegmentProps {
  segment: SegmentSummary;
  segmentContextType: 'invocation' | 'beforeInvocation' | 'afterInvocation';
  isMostRecentSegment?: boolean;
}

export const AndroidHistorySegment = memo(function AndroidHistorySegment({
  segment,
  segmentContextType,
  isMostRecentSegment,
}: AndroidHistorySegmentProps) {
  const formattedRate = getFormattedFailureRate(segment);
  const style = getStatusStyle(getFailureRateStatusType(segment), 'outlined');
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
        <AndroidHistorySegmentTooltip
          segment={segment}
          segmentContextType={segmentContextType}
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
          cursor: 'default',
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

function getFormattedFailureRate(segment: SegmentSummary): string {
  if (!segment.health?.fail_rate) {
    return 'N/A';
  }
  const rate = segment.health.fail_rate.rate || 0;
  return `${(rate * 100).toFixed(0)}%`;
}

function getFailureRateStatusType(segment: SegmentSummary): SemanticStatusType {
  if (!segment.health?.fail_rate) {
    return 'unknown';
  }
  const rate = segment.health.fail_rate.rate || 0;
  const ratePercent = rate * 100;

  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}
