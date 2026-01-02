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
  const health = segment.health;
  const startBuildId = segment.startResult?.buildId;
  const endBuildId = segment.endResult?.buildId;

  const renderRate = (
    label: string,
    rateObj?: { failures: string; total: string; rate: number },
  ) => {
    if (!rateObj) return null;
    const percentage = `${Math.floor(rateObj.rate * 100)}%`;
    return (
      <tr key={label}>
        <td
          style={{ padding: '2px 8px', textAlign: 'left', fontWeight: 'bold' }}
        >
          {label}:
        </td>
        <td style={{ padding: '2px 8px', textAlign: 'right' }}>{percentage}</td>
        <td style={{ padding: '2px 8px', textAlign: 'right' }}>
          ({rateObj.failures}/{rateObj.total})
        </td>
      </tr>
    );
  };

  return (
    <Box sx={{ p: 1 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          width: '100%',
          mb: 2,
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
          <Typography variant="subtitle1" component="div">
            Failure Rate: {formattedRate}
          </Typography>
        </Box>

        <Box sx={{ width: '100%', mb: 2, textAlign: 'center' }}>
          {startBuildId && endBuildId && (
            <Typography variant="caption" display="block">
              <a
                href={`https://android-build.corp.google.com/range_search/cls/from_id/${startBuildId}/to_id/${endBuildId}/`}
                target="_blank"
                rel="noopener noreferrer"
                style={{ color: 'inherit', textDecoration: 'underline' }}
              >
                View Changelist
              </a>
            </Typography>
          )}
        </Box>

        {health && (
          <table style={{ borderCollapse: 'collapse', width: '100%' }}>
            <tbody>
              {renderRate('Overall', health.failRate)}
              {renderRate('Before Retries', health.failRateBeforeRetries)}
              {renderRate('After Retries', health.failRateAfterRetries)}
            </tbody>
          </table>
        )}

        {health && (
          <Box
            sx={{
              mt: 2,
              display: 'flex',
              flexWrap: 'wrap',
              gap: 1,
              justifyContent: 'center',
            }}
          >
            {health.droidGardener && (
              <Typography
                variant="caption"
                sx={{
                  bgcolor: 'warning.light',
                  color: 'warning.contrastText',
                  px: 1,
                  borderRadius: 1,
                }}
              >
                Droid Gardener
              </Typography>
            )}
            {health.demoted && (
              <Typography
                variant="caption"
                sx={{
                  bgcolor: 'error.light',
                  color: 'error.contrastText',
                  px: 1,
                  borderRadius: 1,
                }}
              >
                Demoted
              </Typography>
            )}
          </Box>
        )}
      </Box>

      {segmentContextType === 'invocation' && (
        <Typography variant="body2" sx={{ fontStyle: 'italic', mt: 1 }}>
          Current Invocation
        </Typography>
      )}
      {segmentContextType === 'afterInvocation' && (
        <Typography variant="body2" sx={{ fontStyle: 'italic', mt: 1 }}>
          Newer Segment
        </Typography>
      )}
      {segmentContextType === 'beforeInvocation' && (
        <Typography variant="body2" sx={{ fontStyle: 'italic', mt: 1 }}>
          Older Segment
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
  if (!segment.health?.failRate) {
    return 'N/A';
  }
  const rate = segment.health.failRate.rate || 0;
  return `${(rate * 100).toFixed(0)}%`;
}

function getFailureRateStatusType(segment: SegmentSummary): SemanticStatusType {
  if (!segment.health?.failRate) {
    return 'unknown';
  }
  const rate = segment.health.failRate.rate || 0;
  const ratePercent = rate * 100;

  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}
