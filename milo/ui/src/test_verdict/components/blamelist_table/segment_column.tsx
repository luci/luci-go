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

import { Box, styled, TableCell } from '@mui/material';

import {
  getBackgroundColor,
  getBorderColor,
} from '@/test_verdict/tools/segment_color';

import { useChangepointsWithCommit, useSegmentWithCommit } from './context';

export function SegmentHeadCell() {
  return (
    <TableCell width="1px" colSpan={2}>
      Seg.
    </TableCell>
  );
}

export interface SegmentContentCellProps {
  readonly position: string;
}

export function SegmentContentCell({ position }: SegmentContentCellProps) {
  return (
    <>
      <SegmentSpan position={position} />
      <ChangepointSpan position={position} />
    </>
  );
}

const Span = styled(Box)`
  width: 20px;
  box-sizing: border-box;
  padding-top: 2px;
  padding-bottom: 2px;
  height: 100%;
`;

interface SegmentSpanProps {
  readonly position: string;
}

function SegmentSpan({ position }: SegmentSpanProps) {
  const segment = useSegmentWithCommit(position);
  if (!segment) {
    return (
      <TableCell
        rowSpan={2}
        width="1px"
        sx={{ paddingRight: '2px !important', borderBottom: 'none' }}
      >
        <Span />
      </TableCell>
    );
  }

  if (position !== segment.endPosition) {
    return <></>;
  }

  const span =
    parseInt(segment.endPosition) - parseInt(segment.startPosition) + 1;

  return (
    <TableCell
      rowSpan={span * 2}
      width="1px"
      sx={{
        height: 'inherit',
        paddingRight: '2px !important',
        borderBottom: 'none',
      }}
    >
      <Span
        sx={{
          border: 'solid 1px',
          borderColor: getBorderColor(segment),
          backgroundColor: getBackgroundColor(segment),
        }}
      />
    </TableCell>
  );
}

interface ChangepointSpanProps {
  readonly position: string;
}

function ChangepointSpan({ position }: ChangepointSpanProps) {
  const changepoints = useChangepointsWithCommit(position);

  // TODO: support overlapping changepoints.
  const firstCP = changepoints[0];
  if (!firstCP) {
    return (
      <TableCell
        rowSpan={2}
        width="1px"
        sx={{ paddingLeft: '2px !important', borderBottom: 'none' }}
      >
        <Span />
      </TableCell>
    );
  }

  if (position !== firstCP.startPositionUpperBound99th) {
    return <></>;
  }

  const span =
    parseInt(firstCP.startPositionUpperBound99th) -
    parseInt(firstCP.startPositionLowerBound99th) +
    1;

  return (
    <TableCell
      rowSpan={span * 2}
      width="1px"
      sx={{
        height: 'inherit',
        paddingLeft: '2px !important',
        borderBottom: 'none',
      }}
    >
      <Span
        sx={{
          border: 'solid 1px',
          borderColor: 'var(--canceled-color)',
          backgroundColor: 'var(--canceled-bg-color)',
        }}
      />
    </TableCell>
  );
}
