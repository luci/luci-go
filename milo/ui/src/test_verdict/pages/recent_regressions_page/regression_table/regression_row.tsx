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
import { Link as RouterLink } from 'react-router';

import { OutputChangepointGroupSummary } from '@/analysis/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { Timestamp } from '@/common/components/timestamp';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { getGitilesCommitURL } from '@/gitiles/tools/utils';

import { useDetailsUrlPath } from './context';
import { FailureRatePieChart } from './failure_rate_pie_chart';

const TextCell = styled(TableCell)({
  whiteSpace: 'nowrap',
  padding: '0 10px',
});

const NumberCell = styled(TableCell)({
  fontWeight: 'bold',
  textAlign: 'center',
  padding: 0,
});

const ChartCell = styled(TableCell)({
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
  const detailsUrlPath = useDetailsUrlPath(regression);

  const blamelistCommitCount =
    parseInt(canonicalChangepoint.startPositionUpperBound99th) -
    parseInt(canonicalChangepoint.startPositionLowerBound99th) +
    1;

  return (
    <TableRow>
      <TextCell>
        <Timestamp
          datetime={DateTime.fromISO(canonicalChangepoint.startHour)}
          format="MMM dd, HH:mm"
          extraTimezones={{ format: SHORT_TIME_FORMAT }}
        />
      </TextCell>
      <TextCell>
        {/* TODO(b/321110247): link to the blamelist instead of the branch. */}
        <HtmlTooltip
          title={
            <>
              <h4>UNIMPLEMENTED</h4>
              <p>
                For now, this link will only take you to the git logs of the
                blamelist branch. Use the details link instead.
              </p>
            </>
          }
        >
          <Link
            href={getGitilesCommitURL(canonicalChangepoint.ref.gitiles)}
            target="_blank"
            rel="noopener"
          >
            {blamelistCommitCount} commits
          </Link>
        </HtmlTooltip>
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
        {statistics.unexpectedVerdictRateChange.countIncreased50To100Percent ||
          ''}
      </NumberCell>
      <NumberCell sx={{ color: 'var(--warning-color)' }}>
        {statistics.unexpectedVerdictRateChange.countIncreased20To50Percent ||
          ''}
      </NumberCell>
      <NumberCell sx={{ color: 'var(--success-color)' }}>
        {statistics.unexpectedVerdictRateChange.countIncreased0To20Percent ||
          ''}
      </NumberCell>

      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their failed source verdict rate before the changepoint"
          buckets={statistics.unexpectedVerdictRateBefore.buckets}
        />
      </ChartCell>
      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their failed source verdict rate after the changepoint"
          buckets={statistics.unexpectedVerdictRateAfter.buckets}
        />
      </ChartCell>
      <ChartCell>
        <FailureRatePieChart
          label="The test variants broken down by their recent failed source verdict rate"
          buckets={statistics.unexpectedVerdictRateCurrent.buckets}
        />
      </ChartCell>
      <TextCell>
        <Link component={RouterLink} to={detailsUrlPath}>
          details
        </Link>
      </TextCell>
      <TextCell sx={{ overflow: 'hidden' }}>
        <div css={{ display: 'inline-grid', gridTemplateColumns: '1fr auto' }}>
          <span
            title={canonicalChangepoint.testId}
            css={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {canonicalChangepoint.testId}
          </span>
          <CopyToClipboard
            textToCopy={canonicalChangepoint.testId}
            title="Copy test ID."
          />
        </div>
      </TextCell>
    </TableRow>
  );
}
