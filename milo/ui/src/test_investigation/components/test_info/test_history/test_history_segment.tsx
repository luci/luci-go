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

import { Box, Button, ButtonGroup, Link, Typography } from '@mui/material';

import { getStatusStyle } from '@/common/styles/status_styles';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { useProject, useTestVariant } from '@/test_investigation/context';
import {
  getFailureRateStatusTypeFromSegment,
  getFormattedFailureRateFromSegment,
} from '@/test_investigation/utils/test_history_utils';

import { useTestVariantBranch } from '../context';

interface TestHistorySegmentProps {
  segment: Segment;
}

export function TestHistorySegment({ segment }: TestHistorySegmentProps) {
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

  const formattedRate = getFormattedFailureRateFromSegment(segment);

  const style = getStatusStyle(
    getFailureRateStatusTypeFromSegment(segment),
    'outlined',
  );
  const IconComponent = style.icon;

  return (
    <Box
      sx={{
        display: 'inline-grid',
        gridTemplateColumns: '1fr',
        gap: 1,
      }}
    >
      <ButtonGroup
        sx={{
          display: 'flex',
          width: '100%',
          gap: '10px',
        }}
      >
        <Button
          variant="contained"
          sx={{
            marginRight: 'auto',
            backgroundColor: 'grey.100',
            display: 'flex',
            flexDirection: 'column',
            borderRadius: 0,
            minWidth: '82px',
            minHeight: '72px',
            padding: '10px 8px',
            '&:hover': {
              backgroundColor: 'grey.200',
            },
            alignItems: 'flex-start',
          }}
        >
          <Typography
            variant="subtitle2"
            component={Link}
            sx={{ textDecoration: 'none', fontWeight: 'normal' }}
          >
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
            <>
              <Typography
                variant="caption"
                sx={{ color: 'text.secondary', fontStyle: 'italic' }}
              >
                {new Date(segment.endHour).toLocaleDateString()}
              </Typography>
              <Typography
                variant="caption"
                sx={{ color: 'text.secondary', fontStyle: 'italic' }}
              >
                {new Date(segment.endHour).toLocaleTimeString()}
              </Typography>
            </>
          )}
        </Button>
        <Button
          variant="contained"
          sx={{
            marginLeft: 'auto',
            backgroundColor: 'grey.100',
            display: 'flex',
            flexDirection: 'column',
            borderRadius: 0,
            minWidth: '82px',
            minHeight: '72px',
            padding: '10px 8px',
            '&:hover': {
              backgroundColor: 'grey.200',
            },
            alignItems: 'flex-start',
          }}
        >
          <Typography
            variant="subtitle2"
            component={Link}
            sx={{ textDecoration: 'none', fontWeight: 'normal' }}
          >
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
            <>
              <Typography
                variant="caption"
                sx={{ color: 'text.secondary', fontStyle: 'italic' }}
              >
                {new Date(segment.startHour).toLocaleDateString()}
              </Typography>
              <Typography
                variant="caption"
                sx={{ color: 'text.secondary', fontStyle: 'italic' }}
              >
                {new Date(segment.startHour).toLocaleTimeString()}
              </Typography>
            </>
          )}
        </Button>
      </ButtonGroup>
      <Box
        sx={{
          backgroundColor: style.backgroundColor,
          display: 'flex',
          flexDirection: 'row',
          borderRadius: '100px',
          padding: '4px 8px',
          boxShadow: 0,
          gap: '4px',
          justifyContent: 'center',
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
              (
              <Typography
                sx={{
                  color: 'primary.main',
                  fontStyle: 'italic',
                }}
                variant="caption"
              >
                {segment?.counts.unexpectedFailedResults}/
                {segment?.counts.totalResults} failed
              </Typography>
              )
            </Typography>
          </>
        )}
      </Box>
    </Box>
  );
}
