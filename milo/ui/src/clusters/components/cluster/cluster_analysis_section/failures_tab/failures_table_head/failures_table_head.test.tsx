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

import '@testing-library/jest-dom';

import { render, screen } from '@testing-library/react';

import { identityFunction } from '@/clusters/testing_tools/functions';

import FailuresTableHead from './failures_table_head';

describe('Test FailureTableHead', () => {
  it('should display sortable table head', async () => {
    render(
      <table>
        <FailuresTableHead
          isAscending={false}
          toggleSort={identityFunction}
          sortMetric={'latestFailureTime'}
        />
      </table>,
    );

    await screen.findByTestId('failure_table_head');

    expect(screen.getByText('User CLs Failed Presubmit')).toBeInTheDocument();
    expect(screen.getByText('Builds Failed')).toBeInTheDocument();
    expect(
      screen.getByText('Presubmit-Blocking Failures Exonerated'),
    ).toBeInTheDocument();
    expect(screen.getByText('Total Failures')).toBeInTheDocument();
    expect(screen.getByText('Latest Failure Time')).toBeInTheDocument();
  });
});
