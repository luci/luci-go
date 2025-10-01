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
import { Segment_Counts } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { StartPointInfo } from '@/test_verdict/components/changepoint_analysis';

import {
  START_POINT_SPAN_HEIGHT,
  SPAN_MARGIN,
  SEGMENT_SPAN_HEIGHT,
} from '../constants';
import { useBlamelistDispatch, useConfig } from '../context';

const Span = styled(Box)`
  container-type: inline-size;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  grid-gap: 5px;
  color: var(--canceled-color);
  font-weight: bold;
  margin: 0px ${SPAN_MARGIN}px;
  width: calc(100% - ${2 * SPAN_MARGIN}px);
  height: 100%;
  text-align: center;
  cursor: pointer;
`;

const SPAN_LINE_WIDTH = 4;

const SpanLine = styled(Box)`
  position: relative;
  top: 50%;
  transform: translateY(-50%);

  min-width: 14px;
  height: ${SPAN_LINE_WIDTH}px;
  background-color: var(--canceled-bg-color);

  &::after {
    display: block;
    content: '';
    width: ${SPAN_LINE_WIDTH}px;
    height: ${(START_POINT_SPAN_HEIGHT - SPAN_LINE_WIDTH) / 2 - 1}px;
    background-color: var(--canceled-bg-color);
  }
  &:last-of-type::after {
    float: right;
  }

  .start-point-top > &::after {
    transform: translateY(${SPAN_LINE_WIDTH}px);
  }
  .start-point-bottom > &::after {
    transform: translateY(-100%);
  }
`;

const SpanText = styled(Box)`
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: ${START_POINT_SPAN_HEIGHT}px;

  // Elide the label first when there's not enough room to display everything
  // so we don't need to elide the commit count. Long term people will know what
  // the lines mean. The actual commit count is what matters.
  @container (min-width: 300px) {
    .start-point-fix > &::before {
      content: 'fix candidates: ';
    }
    .start-point-regression > &::before {
      content: 'culprit candidates: ';
    }
  }

  @container (min-width: 100px) {
    &::after {
      content: ' commits';
    }
  }
`;

/**
 * Calculate a weighted failure ratio of a segment. This is used to determine
 * whether a changepoint is a fix or a regression **for UI purpose**.
 *
 * The weights are chosen quite arbitrarily and might be changed in the future.
 */
function weightedFailureRatio(counts: Segment_Counts) {
  return (
    (counts.unexpectedVerdicts + counts.flakyVerdicts / 2) /
    counts.totalVerdicts
  );
}

export interface StartPointSpanProps {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly segment: OutputSegment;
  readonly prevSegment: OutputSegment | null;
  readonly position: 'top' | 'bottom';
}

/**
 * Renders a span that represents the 99th confidence interval of the segment
 * start point.
 */
export function StartPointSpan({
  testVariantBranch,
  segment,
  prevSegment,
  position,
}: StartPointSpanProps) {
  const dispatch = useBlamelistDispatch();
  const { commitMap, xScale, rowHeight } = useConfig();

  // `segment.hasStartChangepoint === true` should imply `prevSegment !== null`.
  // Check both for type narrowing.
  if (!segment.hasStartChangepoint || !prevSegment) {
    return <></>;
  }

  const failureRatio = weightedFailureRatio(segment.counts);
  const prevFailureRatio = weightedFailureRatio(prevSegment.counts);

  const classNames = [
    'start-point-' + position,
    failureRatio < prevFailureRatio
      ? 'start-point-fix'
      : 'start-point-regression',
  ];

  const start = commitMap[segment.startPositionUpperBound99th];
  const end =
    // The lower bound is exclusive, so add one to convert it to an inclusive bound.
    commitMap[(parseInt(segment.startPositionLowerBound99th) + 1).toString()] +
    // Finally, add +1 so we draw a line to the end of the cell, not the start of the cell.
    1;
  const y =
    rowHeight / 2 +
    (position === 'top'
      ? -SEGMENT_SPAN_HEIGHT / 2 - START_POINT_SPAN_HEIGHT
      : SEGMENT_SPAN_HEIGHT / 2);
  const commitCount =
    parseInt(segment.startPositionUpperBound99th) -
    parseInt(segment.startPositionLowerBound99th);

  return (
    <foreignObject
      x={xScale(start)}
      y={y}
      width={xScale(end) - xScale(start) + 1}
      height={START_POINT_SPAN_HEIGHT}
    >
      <HtmlTooltip
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
          className={classNames.join(' ')}
          onClick={() =>
            dispatch({
              type: 'showBlamelist',
              testVariantBranch,
              focusCommitPosition: segment.startPositionUpperBound99th,
            })
          }
        >
          <SpanLine />
          <SpanText>{commitCount}</SpanText>
          <SpanLine />
        </Span>
      </HtmlTooltip>
    </foreignObject>
  );
}
