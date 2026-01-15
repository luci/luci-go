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

import { CheckCircle } from '@mui/icons-material';
import ErrorIcon from '@mui/icons-material/Error';
import { Box, Button, ButtonGroup, Link, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { ParsedTestVariantBranchName } from '@/analysis/types';
import { useTestVariantBranchesClient } from '@/common/hooks/prpc_clients';
import { getStatusStyle } from '@/common/styles/status_styles';
import {
  QuerySourceVerdictsRequest,
  QuerySourceVerdictsResponse_VerdictStatus,
  Segment,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { useProject, useTestVariant } from '@/test_investigation/context';
import {
  getFailureRateStatusTypeFromSegment,
  getFormattedFailureRateFromSegment,
} from '@/test_investigation/utils/test_history_utils';

import { useTestVariantBranch } from '../context';

interface TestHistorySegmentProps {
  segment: Segment;
  isStartSegment: boolean;
  isEndSegment: boolean;
}

export function TestHistorySegment({
  segment,
  isStartSegment,
  isEndSegment,
}: TestHistorySegmentProps) {
  const testVariant = useTestVariant();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();
  const tvbClient = useTestVariantBranchesClient();

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

  const endPos = Number(segment.endPosition);
  const endPosEnd = Number(segment.endPosition) - 1000;

  const { data: response } = useQuery({
    ...tvbClient.QuerySourceVerdicts.query(
      QuerySourceVerdictsRequest.fromPartial({
        parent: ParsedTestVariantBranchName.toString(testVariantBranch!),
        startSourcePosition: endPos.toString(),
        endSourcePosition: endPosEnd.toString(),
      }),
    ),
  });

  const startPos = Number(segment.startPosition) + 999;
  const startPosEnd = Number(segment.startPosition) - 1;

  const { data: response2 } = useQuery({
    ...tvbClient.QuerySourceVerdicts.query(
      QuerySourceVerdictsRequest.fromPartial({
        parent: ParsedTestVariantBranchName.toString(testVariantBranch!),
        startSourcePosition: startPos.toString(),
        endSourcePosition: startPosEnd.toString(),
      }),
    ),
  });

  const endSourceVerdicts = response?.sourceVerdicts;
  const startSourceVerdicts = response2?.sourceVerdicts;

  const getEndSourceVerdictFromPosition = (pos: string) => {
    if (!endSourceVerdicts) return null;
    for (let i = 0; i < endSourceVerdicts.length; i++) {
      if (endSourceVerdicts[i].position === pos) {
        return endSourceVerdicts[i];
      }
    }
    return null;
  };

  const getStartSourceVerdictFromPosition = (pos: string) => {
    if (!startSourceVerdicts) return null;
    for (let i = startSourceVerdicts.length - 1; i > 0; i--) {
      if (startSourceVerdicts[i].position === pos) {
        return startSourceVerdicts[i];
      }
    }
    return null;
  };

  const passSourceVerdictBox = (
    <Box
      sx={{
        backgroundColor: 'var(--gm3-color-success)',
        display: 'flex',
        borderRadius: 0,
        minWidth: '82px',
        minHeight: '72px',
        width: '82px',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 0,
      }}
    >
      <Typography
        variant="subtitle1"
        sx={{
          color: 'white',
          fontWeight: 'bold',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '4px',
        }}
      >
        <CheckCircle sx={{ width: '20px' }} /> P
      </Typography>
    </Box>
  );

  const failSourceVerdictBox = (
    <Box
      sx={{
        backgroundColor: 'var(--gm3-color-error)',
        display: 'flex',
        borderRadius: 0,
        minWidth: '82px',
        minHeight: '72px',
        width: '82px',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 0,
      }}
    >
      <Typography
        variant="subtitle1"
        sx={{
          color: 'white',
          fontWeight: 'bold',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: '4px',
        }}
      >
        <ErrorIcon sx={{ width: '20px' }} /> F
      </Typography>
    </Box>
  );

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
          pr: '2px',
        }}
      >
        <Button
          variant="contained"
          sx={{
            backgroundColor: 'grey.100',
            display: 'flex',
            flexDirection: 'column',
            borderRadius: 0,
            padding: 0,
            pl: '10px',
            width: '82px',
            minWidth: '82px',
            minHeight: '72px',
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
                sx={{
                  color: 'text.secondary',
                  fontStyle: 'italic',
                }}
              >
                {new Date(segment.endHour).toLocaleDateString()}
              </Typography>
              <Typography
                variant="caption"
                sx={{
                  color: 'text.secondary',
                  fontStyle: 'italic',
                }}
              >
                {new Date(segment.endHour).toLocaleTimeString()}
              </Typography>
            </>
          )}
        </Button>
        <Button
          variant="contained"
          sx={{
            backgroundColor: 'grey.100',
            display: 'flex',
            flexDirection: 'column',
            borderRadius: 0,
            padding: 0,
            pl: '10px',
            width: '82px',
            minWidth: '82px',
            minHeight: '72px',
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
                {segment?.counts.unexpectedResults}/
                {segment?.counts.totalResults} failed
              </Typography>
              )
            </Typography>
          </>
        )}
      </Box>
      <ButtonGroup
        sx={{
          display: 'flex',
          width: '100%',
          gap: '10px',
          pr: '2px',
        }}
      >
        {getEndSourceVerdictFromPosition(segment.endPosition)?.status ===
        QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED
          ? failSourceVerdictBox
          : passSourceVerdictBox}
        {getStartSourceVerdictFromPosition(segment.startPosition)?.status ===
        QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED
          ? failSourceVerdictBox
          : passSourceVerdictBox}
      </ButtonGroup>
    </Box>
  );
}
