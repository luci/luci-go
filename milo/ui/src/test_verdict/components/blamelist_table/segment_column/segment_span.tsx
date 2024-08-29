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
import { SegmentInfo } from '@/test_verdict/components/changepoint_analysis';
import {
  getBackgroundColor,
  getBorderColor,
} from '@/test_verdict/tools/segment_color';

import { useSegmentWithCommit } from '../context';

import { SEGMENT_SPAN_WIDTH } from './constants';

export const Span = styled(Box)`
  grid-area: middle;

  box-sizing: border-box;
  height: 100%;
  border: solid 1px;

  &.segment-mid {
    border-top: none;
    border-bottom: none;
  }

  &.segment-end {
    margin-top: 2px;
    border-bottom: none;
    border-top-left-radius: ${SEGMENT_SPAN_WIDTH / 2}px;
    border-top-right-radius: ${SEGMENT_SPAN_WIDTH / 2}px;
  }
  &.segment-start {
    margin-bottom: 2px;
    border-top: none;
    border-bottom-left-radius: ${SEGMENT_SPAN_WIDTH / 2}px;
    border-bottom-right-radius: ${SEGMENT_SPAN_WIDTH / 2}px;
  }
`;

export interface SegmentSpanProps {
  readonly position: string;
}

export function SegmentSpan({ position }: SegmentSpanProps) {
  const segment = useSegmentWithCommit(position);

  if (!segment) {
    return <></>;
  }

  const classNames: string[] = [];
  if (segment.startPosition === position) {
    classNames.push('segment-start');
  }
  if (segment.endPosition === position) {
    classNames.push('segment-end');
  }
  if (segment.startPosition !== position && segment.endPosition !== position) {
    classNames.push('segment-mid');
  }

  return (
    <HtmlTooltip disableInteractive title={<SegmentInfo segment={segment} />}>
      <Span
        className={classNames.join(' ')}
        sx={{
          borderColor: getBorderColor(segment),
          backgroundColor: getBackgroundColor(segment),
        }}
      />
    </HtmlTooltip>
  );
}
