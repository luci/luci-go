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

import { Box, Icon } from '@mui/material';

import { OutputSegment, OutputTestVariantBranch } from '@/analysis/types';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  VERDICT_STATUS_COLOR_MAP,
  VERDICT_STATUS_ICON_FONT_MAP,
} from '@/test_verdict/constants/verdict';
import { useBlamelistDispatch } from '@/test_verdict/pages/regression_details_page/context';
import {
  getBackgroundColor,
  getBorderColor,
} from '@/test_verdict/tools/segment_color';

import { ROW_PADDING, SPAN_MARGIN, SPAN_PADDING } from '../constants';
import { useConfig } from '../context';

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
  const spanWidth = xScale(end) - xScale(start) - 2 * SPAN_MARGIN;
  const spanHeight = 2 * rowUnitHeight - 2 * SPAN_MARGIN;

  return (
    <g transform={`translate(${x}, ${y})`}>
      <rect
        x={SPAN_MARGIN}
        y={SPAN_MARGIN}
        width={spanWidth}
        height={spanHeight}
        stroke={getBorderColor(segment)}
        fill={getBackgroundColor(segment)}
      />
      <foreignObject
        x={SPAN_MARGIN + SPAN_PADDING}
        y={SPAN_MARGIN + SPAN_PADDING}
        width={spanWidth - 2 * SPAN_PADDING}
        height={spanHeight - 2 * SPAN_PADDING}
        css={{ cursor: 'pointer' }}
        onClick={() =>
          dispatch({
            type: 'showBlamelist',
            testVariantBranch,
            focusCommitPosition: segment.endPosition,
          })
        }
      >
        {segment.counts.unexpectedVerdicts ? (
          <Box>
            <Icon
              sx={{
                color: VERDICT_STATUS_COLOR_MAP[TestVariantStatus.UNEXPECTED],
                verticalAlign: 'bottom',
              }}
            >
              {VERDICT_STATUS_ICON_FONT_MAP[TestVariantStatus.UNEXPECTED]}
            </Icon>{' '}
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
            <Icon
              sx={{
                color: VERDICT_STATUS_COLOR_MAP[TestVariantStatus.FLAKY],
                verticalAlign: 'bottom',
              }}
            >
              {VERDICT_STATUS_ICON_FONT_MAP[TestVariantStatus.FLAKY]}
            </Icon>{' '}
            <span css={{ lineHeight: '24px' }}>
              {Math.round(
                (segment.counts.flakyVerdicts / segment.counts.totalVerdicts) *
                  100,
              )}
              % ({segment.counts.flakyVerdicts}/{segment.counts.totalVerdicts})
            </span>
          </Box>
        ) : (
          <></>
        )}
      </foreignObject>
    </g>
  );
}
