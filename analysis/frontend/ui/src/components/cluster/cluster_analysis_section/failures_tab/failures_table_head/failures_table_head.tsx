// Copyright 2022 The LUCI Authors.
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

import { styled } from '@mui/material/styles';
import TableCell, { tableCellClasses } from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TableSortLabel from '@mui/material/TableSortLabel';

import { MetricName } from '@/tools/failures_tools';

const NarrowTableCell = styled(TableCell)(() => ({
  [`&.${tableCellClasses.root}`]: {
    padding: '6px 6px',
  },
}));

interface Props {
    toggleSort: (metric: MetricName) => void,
    sortMetric: MetricName,
    isAscending: boolean,
}

const FailuresTableHead = ({
  toggleSort,
  sortMetric,
  isAscending,
}: Props) => {
  return (
    <TableHead data-testid="failure_table_head">
      <TableRow>
        <NarrowTableCell
          sx={{ padding: '10px' }}
        >
        </NarrowTableCell>
        <NarrowTableCell sx={{ width: '160px' }}>Build</NarrowTableCell>
        <NarrowTableCell sx={{ width: '80px' }}>Verdict</NarrowTableCell>
        <NarrowTableCell sx={{ width: '40%' }}>Variant</NarrowTableCell>
        <NarrowTableCell>CL(s)</NarrowTableCell>
        <NarrowTableCell>Presubmit Run</NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'presubmitRejects' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer' }}>
          <TableSortLabel
            aria-label="Sort by User CLs failed Presubmit"
            active={sortMetric === 'presubmitRejects'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('presubmitRejects')}>
              User CLs Failed Presubmit
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'invocationFailures' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer' }}>
          <TableSortLabel
            active={sortMetric === 'invocationFailures'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('invocationFailures')}>
              Builds Failed
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'criticalFailuresExonerated' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer' }}>
          <TableSortLabel
            active={sortMetric === 'criticalFailuresExonerated'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('criticalFailuresExonerated')}>
              Presubmit-Blocking Failures Exonerated
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'failures' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer' }}>
          <TableSortLabel
            active={sortMetric === 'failures'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('failures')}>
              Total Failures
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'latestFailureTime' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer' }}>
          <TableSortLabel
            active={sortMetric === 'latestFailureTime'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('latestFailureTime')}>
              Latest Failure Time
          </TableSortLabel>
        </NarrowTableCell>
      </TableRow>
    </TableHead>
  );
};

export default FailuresTableHead;
