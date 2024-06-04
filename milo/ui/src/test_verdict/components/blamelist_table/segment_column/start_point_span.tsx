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
import { StartPointInfo } from '@/test_verdict/components/changepoint_analysis';

import { useStartPointsWithCommit } from '../context';

import { Span } from './common';

export interface StartPointSpanProps {
  readonly position: string;
}

export function StartPointSpan({ position }: StartPointSpanProps) {
  const startPoints = useStartPointsWithCommit(position);

  // TODO: support overlapping start points.
  const firstSP = startPoints[0];

  return (
    <TableCell
      rowSpan={2}
      width="1px"
      sx={{
        height: 'inherit',
        paddingLeft: '2px !important',
        borderBottom: 'none',
      }}
    >
      {firstSP ? (
        <HtmlTooltip
          arrow
          disableInteractive
          title={<StartPointInfo segment={firstSP} />}
        >
          <Span
            sx={{
              border: 'solid 1px',
              borderTop:
                firstSP.startPositionUpperBound99th === position
                  ? undefined
                  : 'none',
              borderBottom:
                firstSP.startPositionLowerBound99th === position
                  ? undefined
                  : 'none',
              borderColor: 'var(--canceled-color)',
              backgroundColor: 'var(--canceled-bg-color)',
            }}
          />
        </HtmlTooltip>
      ) : (
        <Span />
      )}
    </TableCell>
  );
}
