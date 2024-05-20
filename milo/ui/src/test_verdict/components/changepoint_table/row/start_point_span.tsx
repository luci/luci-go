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

import { OutputTestVariantBranch } from '@/analysis/types';
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { useBlamelistDispatch } from '@/test_verdict/pages/regression_details_page/context';

import { ROW_PADDING, SPAN_MARGIN } from '../constants';
import { useConfig } from '../context';

export interface StartPointSpanProps {
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly segment: Segment;
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

  const start = commitMap[segment.startPositionUpperBound99th] + 1;
  const end = commitMap[segment.startPositionLowerBound99th] + 2;
  const rowUnitHeight = (rowHeight - 2 * ROW_PADDING) / 3;

  return (
    <rect
      x={xScale(start) + SPAN_MARGIN}
      y={ROW_PADDING + SPAN_MARGIN}
      width={xScale(end) - xScale(start) - 2 * SPAN_MARGIN}
      height={rowUnitHeight - 2 * SPAN_MARGIN}
      stroke="#08aaff"
      fill="#b3e5ff"
      css={{ cursor: 'pointer' }}
      onClick={() =>
        dispatch({
          type: 'showBlamelist',
          testVariantBranch,
          focusCommitPosition: segment.startPositionUpperBound99th,
        })
      }
    />
  );
}
