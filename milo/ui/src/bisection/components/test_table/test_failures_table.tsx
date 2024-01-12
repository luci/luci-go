// Copyright 2023 The LUCI Authors.
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

import './test_failures_table.css';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import { DateTime } from 'luxon';

import { Timestamp } from '@/common/components/timestamp';
import { TestFailure } from '@/common/services/luci_bisection';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { getTestHistoryURLPath } from '@/common/tools/url_utils';
import { urlSetSearchQueryParam } from '@/generic_libs/tools/utils';

interface TestFailuresTableProps {
  testFailures?: TestFailure[];
}

export function TestFailuresTable({ testFailures }: TestFailuresTableProps) {
  if (!testFailures?.length) {
    return (
      <span className="data-placeholder">Error: No test failures found</span>
    );
  }
  return (
    <TableContainer component={Paper} className="test-failures-table-container">
      <Table
        className="test-failures-table"
        size="small"
        data-testid="test-failures-table"
      >
        <TableHead>
          <TableRow>
            <TableCell>Test ID</TableCell>
            <TableCell>Test suite</TableCell>
            <TableCell>Failure start hour</TableCell>
            <TableCell>Diverged</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {testFailures.map((t) => (
            <TestFailureRow
              key={`${t.testId}-${t.variantHash}`}
              testFailure={t}
            />
          ))}
        </TableBody>
        {/* TODO: display more information about the test failure. eg the rerun test results. */}
      </Table>
    </TableContainer>
  );
}

interface TestFailureRowProps {
  testFailure: TestFailure;
}

function TestFailureRow({ testFailure }: TestFailureRowProps) {
  // TODO: get project from test failure and remove the hardcoded project.
  const testHistoryLink = urlSetSearchQueryParam(
    getTestHistoryURLPath('chromium', testFailure.testId),
    'q',
    `VHash:${testFailure.variantHash}`,
  );
  return (
    <>
      <TableRow data-testid="test_failure_row">
        <TableCell sx={{ overflowWrap: 'anywhere' }}>
          {testFailure.testId} -
          <Link
            href={testHistoryLink}
            target="_blank"
            rel="noreferrer"
            underline="always"
          >
            History
          </Link>
        </TableCell>
        <TableCell>
          {testFailure.variant.def['test_suite'] || 'unavailable'}
        </TableCell>
        <TableCell>
          <Timestamp
            datetime={DateTime.fromISO(testFailure.startHour)}
            format={SHORT_TIME_FORMAT}
          />
        </TableCell>
        <TableCell>{testFailure.isDiverged ? 'True' : 'False'}</TableCell>
      </TableRow>
    </>
  );
}
