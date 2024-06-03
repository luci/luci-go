// Copyright 2024 The LUCI Authors.
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

import { Box, styled } from '@mui/material';

import { OutputSegment, OutputTestVariantBranch } from '@/analysis/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { SegmentInfo } from '@/test_verdict/components/changepoint_analysis';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { useBlamelistDispatch } from '@/test_verdict/pages/regression_details_page/context';
import {
  getBackgroundColor,
  getBorderColor,
} from '@/test_verdict/tools/segment_color';

import { ROW_PADDING, SPAN_MARGIN } from '../constants';
import { useConfig } from '../context';

const Span = styled(Box)`
  border: solid 1px;
  box-sizing: border-box;
  margin: ${SPAN_MARGIN}px;
  width: calc(100% - ${2 * SPAN_MARGIN}px);
  height: calc(100% - ${2 * SPAN_MARGIN}px);
  cursor: pointer;
`;

export interface SegmentSpanProps {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly segment: OutputSegment;
}

/**
 * Renders a span that represents the segment.
 */
export function SegmentSpan({ testVariantBranch, segment }: SegmentSpanProps) {
  const dispatch = useBlamelistDispatch();
  const { commitMap, xScale, rowHeight } = useConfig();

  const start = commitMap[segment.endPosition];
  const end = commitMap[segment.startPosition] + 1;

  const rowUnitHeight = (rowHeight - 2 * ROW_PADDING) / 3;
  const x = xScale(start);
  const y = ROW_PADDING + rowUnitHeight;
  const spanWidth = xScale(end) - xScale(start);
  const spanHeight = 2 * rowUnitHeight;

  return (
    <foreignObject
      x={x}
      y={y}
      width={spanWidth}
      height={spanHeight}
      css={{ cursor: 'pointer' }}
      onClick={() =>
        dispatch({
          type: 'showBlamelist',
          testVariantBranch,
          focusCommitPosition: segment.endPosition,
        })
      }
    >
      <HtmlTooltip
        arrow
        disableInteractive
        title={
          <SegmentInfo
            segment={segment}
            instructionRow={
              <tr>
                <td colSpan={100}>
                  <Box sx={{ marginBottom: '5px', fontWeight: 'bold' }}>
                    Click to view blamelist with test results.
                  </Box>
                </td>
              </tr>
            }
          />
        }
      >
        <Span
          sx={{
            backgroundColor: getBackgroundColor(segment),
            borderColor: getBorderColor(segment),
          }}
        >
          {segment.counts.unexpectedVerdicts ? (
            <Box>
              <VerdictStatusIcon
                status={TestVariantStatus.UNEXPECTED}
                sx={{ verticalAlign: 'bottom' }}
              />{' '}
              <span css={{ lineHeight: '24px' }}>
                {Math.round(
                  (segment.counts.unexpectedVerdicts /
                    segment.counts.totalVerdicts) *
                    100,
                )}
                % ({segment.counts.unexpectedVerdicts}/
                {segment.counts.totalVerdicts})
              </span>
            </Box>
          ) : (
            <></>
          )}
          {segment.counts.flakyVerdicts ? (
            <Box>
              <VerdictStatusIcon
                status={TestVariantStatus.FLAKY}
                sx={{ verticalAlign: 'bottom' }}
              />{' '}
              <span css={{ lineHeight: '24px' }}>
                {Math.round(
                  (segment.counts.flakyVerdicts /
                    segment.counts.totalVerdicts) *
                    100,
                )}
                % ({segment.counts.flakyVerdicts}/{segment.counts.totalVerdicts}
                )
              </span>
            </Box>
          ) : (
            <></>
          )}
        </Span>
      </HtmlTooltip>
    </foreignObject>
  );
}
