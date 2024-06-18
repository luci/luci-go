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

import { Box, TableCell, styled } from '@mui/material';

import { SEGMENT_SPAN_WIDTH, START_POINT_SPAN_WIDTH } from './constants';
import { SegmentSpan } from './segment_span';
import { StartPointSpan } from './start_point_span';

const Container = styled(Box)`
  display: grid;
  grid-template-columns: ${START_POINT_SPAN_WIDTH}px ${SEGMENT_SPAN_WIDTH}px ${START_POINT_SPAN_WIDTH}px;
  grid-template-areas: 'left middle right';

  height: 100%;
`;

export function SegmentHeadCell() {
  return <TableCell width="1px">Seg.</TableCell>;
}

export interface SegmentContentCellProps {
  readonly position: string;
}

export function SegmentContentCell({ position }: SegmentContentCellProps) {
  return (
    <TableCell
      width="1px"
      sx={{
        height: 'inherit',
        borderBottom: 'none',
      }}
      rowSpan={2}
    >
      <Container>
        <SegmentSpan position={position} />
        <StartPointSpan position={position} />
      </Container>
    </TableCell>
  );
}
