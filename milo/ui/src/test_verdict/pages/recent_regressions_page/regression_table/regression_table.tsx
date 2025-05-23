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

import { Help } from '@mui/icons-material';
import {
  Table as MuiTable,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  styled,
} from '@mui/material';

import { OutputChangepointGroupSummary } from '@/analysis/types';

import { RegressionTableContextProvider } from './context';
import { RegressionRow } from './regression_row';
import { GetDetailsUrlPath } from './types';

const AVG_FAILURE_RATE_TOOLTIP = `\
  The average of the average failed source verdict rate of the test variants in the segments.\
`;

const FAILURE_RATE_DELTA_TOOLTIP = `\
  The number of test variants in each failure rate increase bucket. This is useful for gauging the impact of this \
  regression.\
`;

const BEFORE_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are failing \
  (red, >95% recent failed source verdict rate), \
  have mixed results (yellow, 5~95% recent failed source verdict rate), \
  or passing (green, <5% recent failed source verdict rate) before the changepoint.\
`;

const AFTER_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are still failing \
  (red, >95% recent failed source verdict rate), \
  have mixed results (yellow, 5~95% recent failed source verdict rate), \
  or have been fixed (green, <5% recent failed source verdict rate) after the changepoint.\
`;

const CURRENT_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are still failing \
  (red, >95% recent failed source verdict rate), \
  have mixed results (yellow, 5~95% recent failed source verdict rate), \
  or have been fixed (green, <5% recent failed source verdict rate). \
  This is useful for determining whether this regression has been fixed.\
`;

const Table = styled(MuiTable)({
  // Use fixed layout to ensure the table won't grow too wide due to long Test
  // IDs.
  tableLayout: 'fixed',
  '& td,th': {
    // Fix the font size so we can carefully pick the width of each column that
    // works with the fixed layout.
    fontSize: '14px',
  },
  '& thead': {
    backgroundColor: 'var(--block-background-color)',
  },
  '& tbody tr:nth-of-type(2n)': {
    backgroundColor: 'var(--block-background-color)',
  },
  '& th .MuiSvgIcon-root': {
    verticalAlign: 'text-bottom',
  },
});

const TextCell = styled(TableCell)({
  paddingLeft: '10px',
  paddingRight: '10px',
});

const NumberCell = styled(TableCell)({
  textAlign: 'center',
  paddingLeft: '10px',
  paddingRight: '10px',
});

const ChartCell = styled(TableCell)({
  textAlign: 'center',
  paddingLeft: '5px',
  paddingRight: '5px',
});

export interface RegressionTableProps {
  readonly regressions: readonly OutputChangepointGroupSummary[];
  readonly getDetailsUrlPath: GetDetailsUrlPath;
}

export function RegressionTable({
  regressions,
  getDetailsUrlPath,
}: RegressionTableProps) {
  return (
    <RegressionTableContextProvider getDetailsUrlPath={getDetailsUrlPath}>
      <Table width="100%">
        <TableHead>
          <TableRow>
            <TextCell rowSpan={2} width="80px">
              Start Time
            </TextCell>
            <TextCell rowSpan={2} width="80px">
              Blamelist
            </TextCell>
            <NumberCell rowSpan={2} width="40px">
              Test Variant Count
            </NumberCell>
            <TableCell colSpan={3} width="120px" align="center">
              Avg Failure Rate
              <Tooltip title={AVG_FAILURE_RATE_TOOLTIP}>
                <Help fontSize="small" />
              </Tooltip>
            </TableCell>
            <TableCell colSpan={3} width="150px" align="center">
              Failure Rate Increase
              <Tooltip title={FAILURE_RATE_DELTA_TOOLTIP}>
                <Help fontSize="small" />
              </Tooltip>
            </TableCell>
            <ChartCell rowSpan={2} width="40px">
              Before Status
              <Tooltip title={BEFORE_STATUS_TOOLTIP}>
                <Help fontSize="small" />
              </Tooltip>
            </ChartCell>
            <ChartCell rowSpan={2} width="40px">
              After Status
              <Tooltip title={AFTER_STATUS_TOOLTIP}>
                <Help fontSize="small" />
              </Tooltip>
            </ChartCell>
            <ChartCell rowSpan={2} width="40px">
              Current Status
              <Tooltip title={CURRENT_STATUS_TOOLTIP}>
                <Help fontSize="small" />
              </Tooltip>
            </ChartCell>
            <TextCell rowSpan={2} width="40px">
              Details
              <Tooltip
                title={
                  'View the historical segments of all the test variants in this regression.'
                }
              >
                <Help fontSize="small" />
              </Tooltip>
            </TextCell>
            <TextCell rowSpan={2}>Sample Test ID</TextCell>
          </TableRow>
          <TableRow>
            <NumberCell>Before</NumberCell>
            <NumberCell>After</NumberCell>
            <NumberCell>Delta</NumberCell>
            <NumberCell
              sx={{
                color: 'var(--failure-color)',
              }}
            >
              {'≥50%'}
            </NumberCell>
            <NumberCell sx={{ color: 'var(--warning-color)' }}>
              {'20~50%'}
            </NumberCell>
            <NumberCell
              sx={{
                color: 'var(--success-color)',
              }}
            >
              {'<20%'}
            </NumberCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {regressions.map((regression, i) => (
            <RegressionRow key={i} regression={regression} />
          ))}
        </TableBody>
      </Table>
    </RegressionTableContextProvider>
  );
}
