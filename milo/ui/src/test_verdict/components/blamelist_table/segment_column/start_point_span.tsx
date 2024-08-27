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

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { StartPointInfo } from '@/test_verdict/components/changepoint_analysis';

import { useStartPointsWithCommit } from '../context';

import { START_POINT_SPAN_WIDTH } from './constants';

const Span = styled(Box)`
  height: 100%;
  width: ${START_POINT_SPAN_WIDTH};

  &.start-point-left {
    grid-area: left;
  }
  &.start-point-right {
    grid-area: right;
  }
`;

const SPAN_LINE_WIDTH = 4;

const SpanLine = styled(Box)`
  position: relative;
  left: 50%;
  transform: translateX(-50%);

  width: ${SPAN_LINE_WIDTH}px;
  height: 100%;
  background-color: var(--canceled-bg-color);

  .start-point-upper-bound > &::before {
    display: block;
    content: '';
    width: ${(START_POINT_SPAN_WIDTH - SPAN_LINE_WIDTH) / 2 - 1}px;
    height: ${SPAN_LINE_WIDTH}px;
    background-color: var(--canceled-bg-color);
  }
  .start-point-left > &::before {
    transform: translateX(${SPAN_LINE_WIDTH}px);
  }
  .start-point-right > &::before {
    transform: translateX(-100%);
  }

  .start-point-lower-bound > &::after {
    display: block;
    content: '';
    width: ${(START_POINT_SPAN_WIDTH - SPAN_LINE_WIDTH) / 2 - 1}px;
    height: ${SPAN_LINE_WIDTH}px;
    background-color: var(--canceled-bg-color);
    position: relative;
    top: calc(100% - ${SPAN_LINE_WIDTH}px);
  }
  .start-point-left > &::after {
    transform: translateX(${SPAN_LINE_WIDTH}px);
  }
  .start-point-right > &::after {
    transform: translateX(-100%);
  }
`;

export interface StartPointSpanProps {
  readonly position: string;
}

export function StartPointSpan({ position }: StartPointSpanProps) {
  const startPoints = useStartPointsWithCommit(position);

  return (
    <>
      {startPoints.map(([spIndex, sp], i) => {
        const classNames = [
          // Alternate between right and left to avoid start point spans
          // overlapping each other.
          spIndex % 2 === 0 ? 'start-point-right' : 'start-point-left',
        ];

        if (sp.startPositionUpperBound99th === position) {
          classNames.push('start-point-upper-bound');
        }
        if (sp.startPositionLowerBound99th === position) {
          classNames.push('start-point-lower-bound');
        }

        return (
          <HtmlTooltip
            key={i}
            arrow
            disableInteractive
            title={<StartPointInfo segment={sp} />}
          >
            <Span className={classNames.join(' ')}>
              <SpanLine />
            </Span>
          </HtmlTooltip>
        );
      })}
    </>
  );
}
