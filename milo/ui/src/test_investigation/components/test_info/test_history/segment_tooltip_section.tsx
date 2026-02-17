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

import { getStatusStyle } from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { formatSegmentTimestamp } from '@/test_investigation/components/test_info/overview/history_rate_display/util';
import { useProject, useTestVariant } from '@/test_investigation/context';
import { getFailureRateStatusTypeFromSegment } from '@/test_investigation/utils/test_history_utils';

import { useTestVariantBranch } from '../context';

interface SegmentTooltipSectionProps {
  segment: Segment;
  nextSegment?: boolean;
}

export function SegmentTooltipSection({
  segment,
  nextSegment = false,
}: SegmentTooltipSectionProps) {
  const testVariant = useTestVariant();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();

  const testId = testVariant.testId;
  const variantHash = testVariant.variantHash;
  const refHash = testVariantBranch?.refHash;
  const blamelistBaseUrl = refHash
    ? `/ui/labs/p/${project}/tests/${encodeURIComponent(testId)}/variants/${variantHash}/refs/${refHash}/blamelist`
    : undefined;

  const createBlamelistLink = (position: string) => {
    return `${blamelistBaseUrl}?expand=${`CP-${position}`}#CP-${position}`;
  };
  const now = useMemo(() => DateTime.now(), []);

  if (!segment) {
    return <></>;
  }

  const formattedVerdictRate =
    segment && segment.counts
      ? Math.round(
          (segment.counts.unexpectedVerdicts / segment.counts.totalVerdicts) *
            100,
        )
      : undefined;

  const style = getStatusStyle(
    getFailureRateStatusTypeFromSegment(segment),
    'filled',
  );
  const IconComponent = style.icon;
  const endHourDisplay = formatSegmentTimestamp(segment.endHour, now);

  return (
    <Box
      sx={{
        minWidth: '100px',
        backgroundColor: style.backgroundColor,
        p: 2,
        m: 0,
        clipPath: nextSegment
          ? 'polygon(15px 0%, 100% 0%, 100% 50%, 100% 100%, 15px 100%, 0% 50%)'
          : 'polygon(0% 0%, 100% 0%, calc(100% - 15px) 50%, 100% 100%, 0% 100%, 0% 50%)',
      }}
    >
      {segment.counts && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          {formattedVerdictRate && (
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'middle',
                gap: 0.5,
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
              <Typography
                sx={{ color: 'text.primary', textTransform: 'none' }}
                variant="caption"
              >
                <Typography sx={{ color: 'text.primary' }} variant="caption">
                  {formattedVerdictRate}% {''}
                </Typography>
                (
                <Typography
                  sx={{
                    color: 'primary.main',
                    fontStyle: 'italic',
                  }}
                  variant="caption"
                >
                  {segment?.counts.unexpectedVerdicts}/
                  {segment?.counts.totalVerdicts} runs failed
                </Typography>
                )
              </Typography>
            </Box>
          )}

          <Typography
            sx={{ color: 'text.secondary', fontStyle: 'italic' }}
            variant="caption"
          >
            {segment?.counts.unexpectedVerdicts + segment?.counts.flakyVerdicts}
            /{segment?.counts.totalVerdicts} runs failed before retries
          </Typography>
          <Typography
            sx={{ color: 'text.secondary', fontStyle: 'italic' }}
            variant="caption"
          >
            {segment?.counts.unexpectedVerdicts}/{segment?.counts.totalVerdicts}{' '}
            runs failed after retries
          </Typography>
          <Typography
            sx={{ color: 'text.secondary', fontStyle: 'italic' }}
            variant="caption"
          >
            Started from{' '}
            <Link
              href={createBlamelistLink(segment.endPosition)}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
            >
              {Number(segment.endPosition)}
            </Link>{' '}
            ({endHourDisplay})
          </Typography>
        </Box>
      )}
    </Box>
  );
}
