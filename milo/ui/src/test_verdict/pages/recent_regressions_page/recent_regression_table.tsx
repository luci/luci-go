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
  Box,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  styled,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { useChangepointsClient } from '@/analysis/hooks/prpc_clients';
import { QueryChangepointGroupSummariesRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { OutputChangepointGroupSummary } from '@/test_verdict/types';

import { RegressionRow } from './regression_row';

const AVG_FAILURE_RATE_TOOLTIP = `\
  The average of the average unexpected verdict rate of the test variants in the segments.\
`;

const FAILURE_RATE_DELTA_TOOLTIP = `\
  The number of test variants in each failure rate increase bucket. This is useful for gauging the impact of this \
  regression.\
`;

const BEFORE_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are failing \
  (red, >95% recent unexpected verdict rate), have mixed results (yellow, 5~95% recent unexpected verdict rate), \
  or passing (green, <5% recent unexpected verdict rate) before the changepoint.\
`;

const AFTER_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are still failing \
  (red, >95% recent unexpected verdict rate), have mixed results (yellow, 5~95% recent unexpected verdict rate), \
  or have been fixed (green, <5% recent unexpected verdict rate) after the changepoint.\
`;

const CURRENT_STATUS_TOOLTIP = `\
  The ratio of test variants in the group that are still failing \
  (red, >95% recent unexpected verdict rate), have mixed results (yellow, 5~95% recent unexpected verdict rate), \
  or have been fixed (green, <5% recent unexpected verdict rate). \
  This is useful for determining whether this regression has been fixed.\
`;

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

export interface RecentRegressionTableProps {
  readonly project: string;
}

export function RecentRegressionTable({ project }: RecentRegressionTableProps) {
  const client = useChangepointsClient();
  const { data, isLoading, isError, error } = useQuery(
    client.QueryChangepointGroupSummaries.query(
      QueryChangepointGroupSummariesRequest.fromPartial({
        project,
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  const groupSummaries =
    data.groupSummaries as readonly OutputChangepointGroupSummary[];

  return (
    <Table
      sx={{
        '& thead': {
          backgroundColor: 'var(--block-background-color)',
        },
        '& tbody tr:nth-of-type(2n)': {
          backgroundColor: 'var(--block-background-color)',
        },
        '& th .MuiSvgIcon-root': {
          verticalAlign: 'text-bottom',
        },
      }}
    >
      <TableHead>
        <TableRow>
          <TextCell rowSpan={2}>Start Time</TextCell>
          <TextCell rowSpan={2}>Blamelist</TextCell>
          <NumberCell rowSpan={2}>Test Variant Count</NumberCell>
          <TableCell colSpan={3} align="center">
            Avg Failure Rate
            <Tooltip title={AVG_FAILURE_RATE_TOOLTIP}>
              <Help fontSize="small" />
            </Tooltip>
          </TableCell>
          <TableCell colSpan={3} align="center">
            Failure Rate Increase
            <Tooltip title={FAILURE_RATE_DELTA_TOOLTIP}>
              <Help fontSize="small" />
            </Tooltip>
          </TableCell>
          <ChartCell rowSpan={2}>
            Before Status
            <Tooltip title={BEFORE_STATUS_TOOLTIP}>
              <Help fontSize="small" />
            </Tooltip>
          </ChartCell>
          <ChartCell rowSpan={2}>
            After Status
            <Tooltip title={AFTER_STATUS_TOOLTIP}>
              <Help fontSize="small" />
            </Tooltip>
          </ChartCell>
          <ChartCell rowSpan={2}>
            Current Status
            <Tooltip title={CURRENT_STATUS_TOOLTIP}>
              <Help fontSize="small" />
            </Tooltip>
          </ChartCell>
          <TextCell rowSpan={2}>
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
        {groupSummaries.map((groupSummary, i) => (
          <RegressionRow key={i} regression={groupSummary} />
        ))}
      </TableBody>
    </Table>
  );
}
