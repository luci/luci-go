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

import { TableCell } from '@mui/material';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { SegmentInfo } from '@/test_verdict/components/changepoint_analysis';
import {
  getBackgroundColor,
  getBorderColor,
} from '@/test_verdict/tools/segment_color';

import { useSegmentWithCommit } from '../context';

import { Span } from './common';

export interface SegmentSpanProps {
  readonly position: string;
}

export function SegmentSpan({ position }: SegmentSpanProps) {
  const segment = useSegmentWithCommit(position);

  return (
    <TableCell
      rowSpan={2}
      width="1px"
      sx={{
        height: 'inherit',
        paddingRight: '2px !important',
        borderBottom: 'none',
      }}
    >
      {segment ? (
        <HtmlTooltip
          arrow
          disableInteractive
          title={<SegmentInfo segment={segment} />}
        >
          <Span
            sx={{
              border: 'solid 1px',
              ...(segment.startPosition === position
                ? {}
                : {
                    borderBottom: 'none',
                    borderBottomLeftRadius: 0,
                    borderBottomRightRadius: 0,
                  }),
              ...(segment.endPosition === position
                ? {}
                : {
                    borderTop: 'none',
                    borderTopLeftRadius: 0,
                    borderTopRightRadius: 0,
                  }),
              borderColor: getBorderColor(segment),
              backgroundColor: getBackgroundColor(segment),
            }}
          />
        </HtmlTooltip>
      ) : (
        <Span />
      )}
    </TableCell>
  );
}
