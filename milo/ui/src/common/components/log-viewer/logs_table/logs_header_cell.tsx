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

import {
  Box,
  Grid,
  SxProps,
  TableCell,
  TableSortLabel,
  Theme,
} from '@mui/material';
import { visuallyHidden } from '@mui/utils';
import { ReactNode } from 'react';

import { SortOrder } from '../constants/table_constants';
import { LogsTableEntry } from '../types';

interface Props {
  title?: string;
  label: string;
  sorted?: boolean;
  sortOrder?: SortOrder;
  sortId?: keyof LogsTableEntry;
  onHeaderSort?: (
    event: React.MouseEvent<unknown>,
    property: keyof LogsTableEntry,
  ) => void;
  width?: string;
  sx?: SxProps<Theme>;
  children?: ReactNode;
}

export function LogsHeaderCell({
  title,
  label,
  sorted,
  sortOrder,
  sortId,
  onHeaderSort,
  width,
  sx,
  children,
}: Props) {
  function createSortHandler(property: keyof LogsTableEntry | undefined) {
    if (!property) {
      return () => {};
    }
    return (event: React.MouseEvent<unknown>) => {
      onHeaderSort?.(event, property);
    };
  }

  return (
    <TableCell
      variant="head"
      title={title}
      align="left"
      size="small"
      sortDirection={sorted ? sortOrder : false}
      sx={{
        fontSize: '11px',
        height: '1rem',
        textAlign: 'left',
        pl: 0,
        pb: 0,
        width,
        ...sx,
      }}
    >
      <Grid container item direction="row" rowSpacing={2}>
        <TableSortLabel
          data-testid={`header-${sortId}`}
          active={sorted}
          direction={sorted ? sortOrder : 'asc'}
          onClick={createSortHandler(sortId)}
          disabled={!sorted}
          sx={{
            width,
          }}
        >
          {label}
          {sorted && (
            <Box component="span" sx={visuallyHidden}>
              {sortOrder === 'desc' ? 'sorted descending' : 'sorted ascending'}
            </Box>
          )}
        </TableSortLabel>
        {children}
      </Grid>
    </TableCell>
  );
}
