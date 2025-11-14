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

import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TableSortLabel from '@mui/material/TableSortLabel';

import { SortableField } from '@/clusters/components/cluster/cluster_analysis_section/exonerations_v2_tab/model/model';
import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';

interface Props {
  toggleSort: (metric: SortableField) => void;
  sortField: SortableField;
  isAscending: boolean;
}

const ExonerationsTableHead = ({
  toggleSort,
  sortField,
  isAscending,
}: Props) => {
  return (
    <TableHead data-testid="exonerations_table_head">
      <TableRow>
        <TableCell
          sortDirection={
            sortField === 'testId' ? (isAscending ? 'asc' : 'desc') : false
          }
          sx={{ cursor: 'pointer' }}
        >
          <TableSortLabel
            active={sortField === 'testId'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('testId')}
          >
            Test
          </TableSortLabel>
        </TableCell>
        <TableCell>Variant</TableCell>
        <TableCell
          sortDirection={
            sortField === 'sourceRef' ? (isAscending ? 'asc' : 'desc') : false
          }
          sx={{ cursor: 'pointer' }}
        >
          <TableSortLabel
            active={sortField === 'sourceRef'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('sourceRef')}
          >
            Branch
          </TableSortLabel>
        </TableCell>
        <TableCell>History</TableCell>
        <TableCell
          sortDirection={
            sortField === 'beingExonerated'
              ? isAscending
                ? 'asc'
                : 'desc'
              : false
          }
          sx={{ cursor: 'pointer' }}
          colSpan={2}
        >
          <TableSortLabel
            active={sortField === 'beingExonerated'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('beingExonerated')}
          >
            Is Being Exonerated{' '}
            <HelpTooltip content="Whether the test variant is currently being exonerated (ignored) by presubmit." />
          </TableSortLabel>
        </TableCell>
        <TableCell
          sortDirection={
            sortField === 'criticalFailuresExonerated'
              ? isAscending
                ? 'asc'
                : 'desc'
              : false
          }
          sx={{ cursor: 'pointer' }}
        >
          <TableSortLabel
            active={sortField === 'criticalFailuresExonerated'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('criticalFailuresExonerated')}
          >
            Presubmit-Blocking Failures Exonerated (7 days)
          </TableSortLabel>
        </TableCell>
        <TableCell
          sortDirection={
            sortField === 'lastExoneration'
              ? isAscending
                ? 'asc'
                : 'desc'
              : false
          }
          sx={{ cursor: 'pointer' }}
        >
          <TableSortLabel
            active={sortField === 'lastExoneration'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('lastExoneration')}
          >
            Last Exoneration
          </TableSortLabel>
        </TableCell>
      </TableRow>
    </TableHead>
  );
};

export default ExonerationsTableHead;
