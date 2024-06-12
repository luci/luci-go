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

import { Table, TableBody } from '@mui/material';
import { render, screen } from '@testing-library/react';

import { OutputChangepointGroupSummary } from '@/analysis/types';
import { ChangepointGroupSummary } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RegressionTableContextProvider } from './context';
import { RegressionRow } from './regression_row';

const mockRegression = ChangepointGroupSummary.fromPartial({
  canonicalChangepoint: {
    startPositionUpperBound99th: '100',
    startPositionLowerBound99th: '90',
    nominalStartPosition: '95',
    previousSegmentNominalEndPosition: '93',
    ref: {
      gitiles: {
        host: 'gitiles.com',
        project: 'proj',
        ref: 'branch/a',
      },
    },
  },
  statistics: {
    unexpectedVerdictRateChange: {
      countIncreased0To20Percent: 10,
      countIncreased20To50Percent: 40,
      countIncreased50To100Percent: 50,
    },
    unexpectedVerdictRateAfter: {
      average: 0.6,
      buckets: {
        countAbove5LessThan95Percent: 30,
        countAbove95Percent: 20,
        countLess5Percent: 10,
      },
    },
    unexpectedVerdictRateBefore: {
      average: 0.6,
      buckets: {
        countAbove5LessThan95Percent: 30,
        countAbove95Percent: 20,
        countLess5Percent: 10,
      },
    },
    unexpectedVerdictRateCurrent: {
      average: 0.6,
      buckets: {
        countAbove5LessThan95Percent: 30,
        countAbove95Percent: 20,
        countLess5Percent: 10,
      },
    },
  },
}) as OutputChangepointGroupSummary;

describe('<RegressionRow />', () => {
  it('should calculate the blamelist commit count correctly', () => {
    render(
      <FakeContextProvider>
        <RegressionTableContextProvider
          getDetailsUrlPath={() => '/details-url'}
        >
          <Table>
            <TableBody>
              <RegressionRow regression={mockRegression} />
            </TableBody>
          </Table>
        </RegressionTableContextProvider>
      </FakeContextProvider>,
    );
    expect(screen.getByText('11 commits')).toBeInTheDocument();
  });
});
