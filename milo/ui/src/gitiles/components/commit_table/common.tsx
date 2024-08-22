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

import { Table, TableRow } from '@mui/material';
import { ReactNode } from 'react';

import { useTableRowProps, useTableSx } from './hooks';

export interface StyledTableProps {
  readonly children?: ReactNode;
}

export function StyledTable({ children, ...props }: StyledTableProps) {
  const sx = useTableSx();

  return (
    <Table
      size="small"
      sx={{
        borderCollapse: 'separate',
        '& td, th': {
          padding: '0px 8px',
        },
        minWidth: '1000px',
        ...sx,
      }}
      {...props}
    >
      {children}
    </Table>
  );
}

export interface StyledTableRowProps {
  readonly children?: ReactNode;
}

export function StyledTableRow({ children }: StyledTableRowProps) {
  const props = useTableRowProps();

  return (
    <TableRow
      {...props}
      sx={{
        '& > td': { whiteSpace: 'nowrap' },
        // Add a fixed height to allow children to use `height: 100%`.
        // The actual height of the row will expand to contain the
        // children.
        height: '1px',
      }}
    >
      {children}
    </TableRow>
  );
}
