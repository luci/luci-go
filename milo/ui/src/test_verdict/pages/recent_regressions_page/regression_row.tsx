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

import { Link, TableCell, TableRow, styled } from '@mui/material';
import { DateTime } from 'luxon';

import { Timestamp } from '@/common/components/timestamp';
import { getGitilesCommitURL } from '@/common/tools/gitiles_utils';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { OutputChangepointGroupSummary } from '@/test_verdict/types';

import { FailureRatePieChart } from './failure_rate_pie_chart';

const TextCell = styled(TableCell)({
  width: '1px',
  whiteSpace: 'nowrap',
  padding: '0 10px',
});

const NumberCell = styled(TableCell)({
  width: '1px',
  fontWeight: 'bold',
  textAlign: 'center',
  padding: 0,
});

const ChartCell = styled(TableCell)({
  width: '1px',
  textAlign: 'center',
  padding: 0,
});

export interface RegressionRowProps {
  readonly regression: OutputChangepointGroupSummary;
}

export function RegressionRow({ regression }: RegressionRowProps) {
  const { canonicalChangepoint, statistics } = regression;
  let deltaColor = 'var(--success-color)';
  if (statistics.unexpectedVerdictRateCurrent.average >= 0.2) {
    deltaColor = 'var(--warning-color)';
  }
  if (statistics.unexpectedVerdictRateCurrent.average > 0.5) {
    deltaColor = 'var(--failure-color)';
  }

  return (
    <TableRow>
      <TextCell>
        <Timestamp
          datetime={DateTime.fromISO(canonicalChangepoint.startHour)}
          format="MM-dd HH:mm"
          extra={{ format: SHORT_TIME_FORMAT }}
        />
      </TextCell>
      <TextCell>
        {/* TODO(b/321110247): link to the blamelist instead of the branch. */}
        <Link
          href={getGitilesCommitURL(canonicalChangepoint.ref.gitiles)}
          target="_blank"
          rel="noopener"
        >
          {canonicalChangepoint.nominalStartPosition}..
          {canonicalChangepoint.previousSegmentNominalEndPosition}
        </Link>
      </TextCell>
      <NumberCell>{statistics.count}</NumberCell>

      <NumberCell>
        {Math.round(statistics.unexpectedVerdictRateBefore.average * 100)}%
      </NumberCell>
      <NumberCell>
        {Math.round(statistics.unexpectedVerdictRateAfter.average * 100)}%
      </NumberCell>
      <NumberCell sx={{ color: deltaColor }}>
        {Math.round(statistics.unexpectedVerdictRateCurrent.average * 100)}%
      </NumberCell>

      <NumberCell sx={{ color: 'var(--failure-color)' }}>
        {parseInt(
          statistics.unexpectedVerdictRateChange.countIncreased50To100Percent,
        ) || ''}
      </NumberCell>
      <NumberCell sx={{ color: 'var(--warning-color)' }}>
        {parseInt(
          statistics.unexpectedVerdictRateChange.countIncreased20To50Percent,
        ) || ''}
      </NumberCell>
      <NumberCell sx={{ color: 'var(--success-color)' }}>
        {parseInt(
          statistics.unexpectedVerdictRateChange.countIncreased0To20Percent,
        ) || ''}
      </NumberCell>

      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their unexpected verdict rate before the changepoint"
          buckets={statistics.unexpectedVerdictRateBefore.buckets}
        />
      </ChartCell>
      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their unexpected verdict rate after the changepoint"
          buckets={statistics.unexpectedVerdictRateAfter.buckets}
        />
      </ChartCell>
      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their recent unexpected verdict rate"
          buckets={statistics.unexpectedVerdictRateCurrent.buckets}
        />
      </ChartCell>

      {/* TODO(b/321110247): link to the regression details page. */}
      <TextCell>details</TextCell>
      <TextCell>{canonicalChangepoint.testId}</TextCell>
    </TableRow>
  );
}