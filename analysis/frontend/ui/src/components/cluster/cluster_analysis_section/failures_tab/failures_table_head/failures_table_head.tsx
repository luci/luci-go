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
import { Tooltip } from '@mui/material';

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
          sx={{ padding: '10px', width: '40px' }}
        >
        </NarrowTableCell>
        <NarrowTableCell sx={{ width: '160px' }}>
          <Tooltip title="The ID of the top-level build containing this test result.  The link views this test result in the top-level build.">
            <span>Build</span>
          </Tooltip>
        </NarrowTableCell>
        <NarrowTableCell sx={{ width: '100px' }}>
          <Tooltip title={<>The combined result of all retries of this test variant in this build.  See <a href="http://go/resultdb-concepts#test-verdict" target="_blank">go/resultdb-concepts</a> for more details.</>}>
            <span>Verdict</span>
          </Tooltip>
        </NarrowTableCell>
        <NarrowTableCell>
          <Tooltip title={<>The key value pairs that define the configuration in which the test was run.  Any variant keys that are used for grouping will be omitted. See <a href="http://go/resultdb-concepts#test-variant" target="_blank">go/resultdb-concepts</a> for more details.</>}>
            <span>Variant</span>
          </Tooltip>
        </NarrowTableCell>
        <NarrowTableCell sx={{ width: '80px' }}>
          <Tooltip title="The Gerrit CL ID and patchset number for each CL that was patched in when the test was run.  Will be empty if none were patched in (e.g. postsubmit tests).">
            <span>CL(s)</span>
          </Tooltip>
        </NarrowTableCell>
        <NarrowTableCell sx={{ width: '100px' }}>
          <Tooltip title="The status of the LUCI CV presubmit run that contained this test result.  Submitted means that the run was successful and that the CL was submitted based on this run.">
            <span>Presubmit Run</span>
          </Tooltip>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'presubmitRejects' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '80px' }}>
          <TableSortLabel
            aria-label="Sort by User CLs failed Presubmit"
            active={sortMetric === 'presubmitRejects'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('presubmitRejects')}>
            <Tooltip title="The number of user authored CLs that have been failed by these test results.  The group totals are a count of distinct CLs, so if there are multiple tests that failed a single CL, the multiple tests will only count as 1.">
              <span>User CLs Failed Presubmit</span>
            </Tooltip>
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'invocationFailures' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '80px' }}>
          <TableSortLabel
            active={sortMetric === 'invocationFailures'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('invocationFailures')}>
            <Tooltip title='The number builds that have been failed by these test results.  The group totals are a count of distinct builds, so if there are multiple tests that failed a single build, the multiple tests will only count as 1.'>
              <span>Builds Failed</span>
            </Tooltip>
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'criticalFailuresExonerated' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '80px' }}>
          <TableSortLabel
            active={sortMetric === 'criticalFailuresExonerated'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('criticalFailuresExonerated')}>
            <Tooltip title="The number failed test results that would have blocked a presubmit run that have been exonerated.  The blocking status is independent of whether the run actually passed or failed.  The group totals are a simple sum of the individual failures.">
              <span>Presubmit-Blocking Failures Exonerated</span>
            </Tooltip>
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'failures' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '80px' }}>
          <TableSortLabel
            active={sortMetric === 'failures'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('failures')}>
            <Tooltip title="The total number of failed test results.  The group totals are a simple sum of the individual failures.">
              <span>Total Failures</span>
            </Tooltip>
          </TableSortLabel>
        </NarrowTableCell>
        <NarrowTableCell
          sortDirection={sortMetric === 'latestFailureTime' ? (isAscending ? 'asc' : 'desc') : false}
          sx={{ cursor: 'pointer', width: '100px' }}>
          <TableSortLabel
            active={sortMetric === 'latestFailureTime'}
            direction={isAscending ? 'asc' : 'desc'}
            onClick={() => toggleSort('latestFailureTime')}>
            <Tooltip title="The time of the latest failure that has been ingested and analyzed. The main source of latency is waiting for all of the tests in a run to be complete.">
              <span>Latest Failure Time</span>
            </Tooltip>
          </TableSortLabel>
        </NarrowTableCell>
      </TableRow>
    </TableHead>
  );
};

export default FailuresTableHead;
