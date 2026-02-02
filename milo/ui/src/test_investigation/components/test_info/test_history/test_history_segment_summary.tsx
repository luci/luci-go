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

import { getStatusStyle } from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import {
  getFailureRateStatusTypeFromSegment,
  getFormattedFailureRateFromSegment,
} from '@/test_investigation/utils/test_history_utils';

interface TestHistorySegmentProps {
  segment: Segment;
  isStartSegment: boolean;
  isEndSegment: boolean;
}

export function TestHistorySegmentSummary({
  segment,
  isStartSegment,
  isEndSegment,
}: TestHistorySegmentProps) {
  const formattedRate = getFormattedFailureRateFromSegment(segment);

  const style = getStatusStyle(
    getFailureRateStatusTypeFromSegment(segment),
    'outlined',
  );
  const IconComponent = style.icon;

  return (
    <Box
      sx={{
        backgroundColor: style.backgroundColor,
        display: 'flex',
        flexDirection: 'row',
        borderRadius:
          isStartSegment && isEndSegment
            ? '100px'
            : isStartSegment
              ? '100px 0 0 100px'
              : isEndSegment
                ? '0 100px 100px 0'
                : '0',
        padding: '4px 8px',
        boxShadow: 0,
        gap: '4px',
        justifyContent: 'center',
        clipPath:
          isStartSegment && isEndSegment
            ? ''
            : isStartSegment
              ? 'polygon(0% 0%, 100% 0%, calc(100% - 5px) 50%, 100% 100%, 0% 100%, 0% 50%)'
              : isEndSegment
                ? 'polygon(5px 0%, 100% 0%, 100% 50%, 100% 100%, 5px 100%, 0% 50%)'
                : 'polygon(5px 0%, 100% 0%, calc(100% - 5px) 50%, 100% 100%, 5px 100%, 0% 50%)',
      }}
    >
      {IconComponent && (
        <IconComponent
          sx={{
            fontSize: '18px',
            color: style.iconColor,
          }}
        />
      )}
      {segment.counts && (
        <>
          <Typography sx={{ color: 'text.primary' }} variant="caption">
            {formattedRate}
          </Typography>
          <Typography
            sx={{ color: 'text.primary', textTransform: 'none' }}
            variant="caption"
          >
            <Typography
              sx={{
                color: 'text.secondary',
                fontStyle: 'italic',
              }}
              variant="caption"
            >
              ({segment?.counts.unexpectedResults}/
              {segment?.counts.totalResults} failed)
            </Typography>
          </Typography>
        </>
      )}
    </Box>
  );
}
