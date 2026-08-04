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
import { DateTime } from 'luxon';
import { memo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  SegmentTooltip,
  getFailureRateStatusTypeFromSegmentCount,
  getFormattedFailureRateFromSegmentCount,
} from '@/common/components/segment_tooltip';
import { getStatusStyle } from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

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
  const formattedRate = getFormattedFailureRateFromSegmentCount(segment.counts);
  const style = getStatusStyle(
    getFailureRateStatusTypeFromSegmentCount(segment.counts),
    'outlined',
  );
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
        <SegmentTooltip
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
