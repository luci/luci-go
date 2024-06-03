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
import { StartPointInfo } from '@/test_verdict/components/changepoint_analysis';
import { useBlamelistDispatch } from '@/test_verdict/pages/regression_details_page/context';

import { ROW_PADDING, SPAN_MARGIN } from '../constants';
import { useConfig } from '../context';

const Span = styled(Box)`
  background-color: #b3e5ff;
  border: solid 1px #08aaff;
  box-sizing: border-box;
  margin: ${SPAN_MARGIN}px;
  width: calc(100% - ${2 * SPAN_MARGIN}px);
  height: calc(100% - ${2 * SPAN_MARGIN}px);
  text-align: center;
  cursor: pointer;
`;

export interface StartPointSpanProps {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly segment: OutputSegment;
}

/**
 * Renders a span that represents the 99th confidence interval of the segment
 * start point.
 */
export function StartPointSpan({
  testVariantBranch,
  segment,
}: StartPointSpanProps) {
  const dispatch = useBlamelistDispatch();
  const { commitMap, xScale, rowHeight } = useConfig();

  if (!segment.hasStartChangepoint) {
    return <></>;
  }

  const start = commitMap[segment.startPositionUpperBound99th];
  const end = commitMap[segment.startPositionLowerBound99th] + 1;
  const rowUnitHeight = (rowHeight - 2 * ROW_PADDING) / 3;
  const commitCount =
    parseInt(segment.startPositionUpperBound99th) -
    parseInt(segment.startPositionLowerBound99th) +
    1;

  return (
    <foreignObject
      x={xScale(start)}
      y={ROW_PADDING}
      width={xScale(end) - xScale(start)}
      height={rowUnitHeight}
    >
      <HtmlTooltip
        arrow
        disableInteractive
        title={
          <StartPointInfo
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
          sx={{ lineHeight: `${rowUnitHeight - 2 * SPAN_MARGIN}px` }}
          onClick={() =>
            dispatch({
              type: 'showBlamelist',
              testVariantBranch,
              focusCommitPosition: segment.startPositionUpperBound99th,
            })
          }
        >
          {commitCount} commits
        </Span>
      </HtmlTooltip>
    </foreignObject>
  );
}
