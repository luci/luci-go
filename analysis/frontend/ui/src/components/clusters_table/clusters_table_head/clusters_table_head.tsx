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

import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TableSortLabel from '@mui/material/TableSortLabel';

import { SortableMetricName } from '@/services/cluster';

interface Props {
    toggleSort: (metric: SortableMetricName) => void,
    sortMetric: SortableMetricName,
    isAscending: boolean,
}

const ClustersTableHead = ({
  toggleSort,
  sortMetric,
  isAscending,
}: Props) => {
  return (
    <TableHead data-testid="clusters_table_head">
      <TableRow>
        <TableCell>Cluster</TableCell>
        <TableCell sx={{ width: '150px' }}>Bug</TableCell>
        <TableCell
          sortDirection={sortMetric === 'presubmit_rejects' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '100px' }}>
          <TableSortLabel
            aria-label="Sort by User CLs failed Presubmit"
            active={sortMetric === 'presubmit_rejects'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('presubmit_rejects')}>
              User Cls Failed Presubmit
          </TableSortLabel>
        </TableCell>
        <TableCell
          sortDirection={sortMetric === 'critical_failures_exonerated' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '100px' }}>
          <TableSortLabel
            active={sortMetric === 'critical_failures_exonerated'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('critical_failures_exonerated')}>
              Presubmit-Blocking Failures Exonerated
          </TableSortLabel>
        </TableCell>
        <TableCell
          sortDirection={sortMetric === 'failures' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '100px' }}>
          <TableSortLabel
            active={sortMetric === 'failures'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('failures')}>
              Total Failures
          </TableSortLabel>
        </TableCell>
      </TableRow>
    </TableHead>
  );
};

export default ClustersTableHead;
