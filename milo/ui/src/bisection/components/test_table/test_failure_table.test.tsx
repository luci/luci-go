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

import { getAllByRole, render, screen } from '@testing-library/react';

import { TestFailure } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { TestFailuresTable } from './test_failures_table';

describe('<NthSectionAnalysisTable />', () => {
  test('if all information is displayed', async () => {
    const mockTestFailures = [
      TestFailure.fromPartial({
        testId: 'test1',
        variant: {
          def: { os: 'linux' },
        },
        variantHash: 'variant1',
        isDiverged: false,
        isPrimary: true,
        startHour: '2023-10-05T17:00:00Z',
      }),
      TestFailure.fromPartial({
        testId: 'test2',
        variant: {
          def: { os: 'mac' },
        },
        variantHash: 'variant2',
        isDiverged: true,
        isPrimary: false,
        startHour: '2023-10-05T17:00:00Z',
      }),
    ];
    render(<TestFailuresTable testFailures={mockTestFailures} />);

    // Check the present of all the header
    expect(screen.getByText('Test ID')).toBeInTheDocument();
    expect(screen.getByText('Test suite')).toBeInTheDocument();
    expect(screen.getByText('Failure start hour')).toBeInTheDocument();
    expect(screen.getByText('Diverged')).toBeInTheDocument();

    // Check 2 test failure rows: 1 for the header and 2 for the data rows
    const rerunTable = screen.getByTestId('test-failures-table');
    expect(getAllByRole(rerunTable, 'row')).toHaveLength(3);
  });
});
