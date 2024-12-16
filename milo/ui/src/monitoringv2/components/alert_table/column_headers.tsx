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

import { TableCell, TableSortLabel } from '@mui/material';

import { SortColumn } from '@/monitoringv2/util/alerts';

interface ColumnHeadersProps {
  sortColumn: SortColumn | undefined;
  sortDirection: 'asc' | 'desc';
  clickSortColumn: (column: SortColumn) => void;
}

export const ColumnHeaders = ({
  sortColumn,
  sortDirection,
  clickSortColumn,
}: ColumnHeadersProps) => {
  return (
    <>
      <TableCell>
        <TableSortLabel
          active={sortColumn === 'failure'}
          onClick={() => clickSortColumn('failure')}
          direction={sortDirection}
        >
          Failure
        </TableSortLabel>
      </TableCell>
      <TableCell width="180px">
        <TableSortLabel
          active={sortColumn === 'history'}
          onClick={() => clickSortColumn('history')}
          direction={sortDirection}
        >
          History
        </TableSortLabel>
      </TableCell>
      <TableCell width="120px">First Failure</TableCell>
      <TableCell width="80px">Blamelist</TableCell>
    </>
  );
};
