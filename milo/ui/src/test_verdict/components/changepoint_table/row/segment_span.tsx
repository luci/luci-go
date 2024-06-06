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
import { VerdictSetStatus } from '@/test_verdict/components//verdict_set_status';
import { SegmentInfo } from '@/test_verdict/components/changepoint_analysis';
import { VERDICT_STATUS_COLOR_MAP } from '@/test_verdict/constants/verdict';
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

  & > * {
    display: inline-block;
  }
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

  const spanHeight = (rowHeight - 2 * ROW_PADDING) / 2;
  const x = xScale(start);
  const y = ROW_PADDING + spanHeight;
  const spanWidth = xScale(end) - xScale(start);
  const counts = segment.counts;
  const unexpectedCount = counts.unexpectedVerdicts;
  const flakyCount = counts.flakyVerdicts;
  const expectedCount = counts.totalVerdicts - unexpectedCount - flakyCount;

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
          <VerdictSetStatus
            counts={{
              [TestVariantStatus.UNEXPECTED]: unexpectedCount,
              [TestVariantStatus.FLAKY]: flakyCount,
              [TestVariantStatus.EXPECTED]: expectedCount,
            }}
          />{' '}
          <span css={{ lineHeight: '24px', fontWeight: 'bold' }}>
            <span
              css={
                unexpectedCount
                  ? {
                      color:
                        VERDICT_STATUS_COLOR_MAP[TestVariantStatus.UNEXPECTED],
                    }
                  : {
                      opacity: 0.2,
                    }
              }
            >
              {unexpectedCount}
            </span>
            <span css={{ opacity: 0.2 }}> / </span>
            <span
              css={
                flakyCount
                  ? {
                      color: VERDICT_STATUS_COLOR_MAP[TestVariantStatus.FLAKY],
                    }
                  : {
                      opacity: 0.2,
                    }
              }
            >
              {flakyCount}
            </span>
            <span css={{ opacity: 0.2 }}> / </span>
            <span
              css={
                expectedCount
                  ? {
                      color:
                        VERDICT_STATUS_COLOR_MAP[TestVariantStatus.EXPECTED],
                    }
                  : {
                      opacity: 0.2,
                    }
              }
            >
              {expectedCount}
            </span>
          </span>
        </Span>
      </HtmlTooltip>
    </foreignObject>
  );
}
