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
import ArrowDropUpIcon from '@mui/icons-material/ArrowDropUp';
import { Box, Tooltip, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';

import { ParsedTestVariantBranchName } from '@/analysis/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { useTestVariantBranchesClient } from '@/common/hooks/prpc_clients';
import { getStatusStyle } from '@/common/styles/status_styles';
import {
  QuerySourceVerdictsRequest,
  Segment,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { useInvocation } from '@/test_investigation/context';
import {
  getFailureRateStatusTypeFromSegment,
  getFormattedFailureRateFromSegment,
} from '@/test_investigation/utils/test_history_utils';

import { useTestVariantBranch } from '../context';

import { SegmentTooltipSection } from './segment_tooltip_section';

interface TestHistorySegmentProps {
  segment: Segment;
  nextSegment: Segment;
  isStartSegment: boolean;
  isEndSegment: boolean;
  numShownVerdicts: number;
}

interface SegmentBoxProps {
  segment: Segment;
  nextSegment: Segment;
  isStartSegment: boolean;
  isEndSegment: boolean;
}

function SegmentBox({
  segment,
  nextSegment,
  isStartSegment,
  isEndSegment,
}: SegmentBoxProps) {
  const style = getStatusStyle(
    getFailureRateStatusTypeFromSegment(segment),
    'filled',
  );
  const formattedRate = getFormattedFailureRateFromSegment(segment);
  const IconComponent = style.icon;
  const testVariantBranch = useTestVariantBranch();
  const tvbClient = useTestVariantBranchesClient();

  const { data: response } = useQuery({
    ...tvbClient.QuerySourceVerdicts.query(
      QuerySourceVerdictsRequest.fromPartial({
        parent: ParsedTestVariantBranchName.toString(testVariantBranch!),
        startSourcePosition: (Number(segment?.startPosition) - 1).toString(),
        endSourcePosition: Math.max(
          Number(nextSegment?.endPosition),
          Number(segment?.startPosition) - 1000,
        ).toString(),
      }),
    ),
    enabled: !!nextSegment && !!testVariantBranch,
  });

  return (
    <HtmlTooltip
      sx={{ p: 0 }}
      slotProps={{
        tooltip: {
          sx: {
            p: 1,
          },
        },
      }}
      title={
        <Box sx={{ display: 'flex', flexDirection: 'row', gap: 0, m: 0 }}>
          <SegmentTooltipSection segment={segment}></SegmentTooltipSection>
          {nextSegment && response && response.sourceVerdicts.length > 0 && (
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                width: '80px',
                backgroundColor: 'grey.100',
                clipPath:
                  'polygon(15px 0%, 100% 0%, calc(100% - 15px) 50%, 100% 100%, 15px 100%, 0% 50%)',
              }}
            >
              <Typography sx={{ color: 'primary.main' }} variant="caption">
                {response?.sourceVerdicts.length} CLs
              </Typography>
            </Box>
          )}
          <SegmentTooltipSection
            segment={nextSegment}
            nextSegment
          ></SegmentTooltipSection>
        </Box>
      }
    >
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
    </HtmlTooltip>
  );
}

export function TestHistorySegmentSummary({
  segment,
  nextSegment,
  isStartSegment,
  isEndSegment,
  numShownVerdicts,
}: TestHistorySegmentProps) {
  const invocation = useInvocation() as Invocation;
  const currentTestResultPosition =
    invocation?.sourceSpec?.sources?.gitilesCommit?.position;
  const [_numberShownVerdicts, setNumberShownVerdicts] = useState(0);

  const isCurrentSegment = useMemo(() => {
    if (!currentTestResultPosition || !segment) return false;

    const pos = Number(currentTestResultPosition);
    const start = Number(segment.startPosition);
    const end = Number(segment.endPosition);

    return pos >= start && pos <= end;
  }, [currentTestResultPosition, segment]);

  const ThisResultTooltip = (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        width: '70px',
      }}
    >
      <ArrowDropUpIcon
        sx={{ width: '16px', height: '16px', p: 0 }}
      ></ArrowDropUpIcon>
      <Typography variant="caption" align="center">
        This result on {currentTestResultPosition}
      </Typography>
    </Box>
  );

  // This is to ensure the current test result tooltip to rerender, otherwise it will be positioned incorrectly when collapsing all.
  useEffect(() => {
    setNumberShownVerdicts(numShownVerdicts);
  }, [numShownVerdicts]);

  return (
    <>
      {isCurrentSegment ? (
        <Tooltip
          title={ThisResultTooltip}
          open={false}
          placement="bottom"
          slotProps={{
            popper: {
              modifiers: [
                {
                  name: 'offset',
                  options: {
                    offset: [0, -14],
                  },
                },
              ],
            },
            tooltip: {
              sx: {
                bgcolor: 'transparent',
                color: 'text.secondary',
              },
            },
          }}
        >
          <Box
            sx={{
              '&:hover': {
                cursor: 'pointer',
              },
            }}
          >
            <SegmentBox
              segment={segment}
              nextSegment={nextSegment}
              isStartSegment={isStartSegment}
              isEndSegment={isEndSegment}
            ></SegmentBox>
          </Box>
        </Tooltip>
      ) : (
        <Box
          sx={{
            '&:hover': {
              cursor: 'pointer',
            },
          }}
        >
          <SegmentBox
            segment={segment}
            nextSegment={nextSegment}
            isStartSegment={isStartSegment}
            isEndSegment={isEndSegment}
          ></SegmentBox>
        </Box>
      )}
    </>
  );
}
