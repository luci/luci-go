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
import { getAllByRole, render, screen } from '@testing-library/react';

import { NthSectionAnalysisTable } from './nthsection_analysis_table';

import {
  NthSectionAnalysisResult,
} from '../../services/luci_bisection';


describe('Test NthSectionAnalysisTable component', () => {
  test('if all information is displayed', async () => {
    const mockAnalysis = createMockAnalysis()
    render(<NthSectionAnalysisTable result={mockAnalysis} />);

    // Check the present of all the header
    await screen.findByTestId('nthsection-analysis-detail')
    expect(screen.getAllByText('Status')).toHaveLength(2)
    expect(screen.getAllByText('Start time')).toHaveLength(2)
    expect(screen.getAllByText('End time')).toHaveLength(2)
    expect(screen.getByText('Suspect')).toBeInTheDocument();
    expect(screen.getByText('Commit')).toBeInTheDocument();
    expect(screen.getByText('Run')).toBeInTheDocument();
    expect(screen.getByText('Index')).toBeInTheDocument();

    // Check 3 rerun rows: 1 for the header and 2 for the data rows
    const rerunTable = screen.getByTestId('nthsection-analysis-rerun-table')
    expect(getAllByRole(rerunTable, 'row')).toHaveLength(3)
  });
});

function createMockAnalysis(): NthSectionAnalysisResult {
  return {
    startTime: '2022-09-06T07:13:16.398865Z',
    endTime: '2022-09-06T07:13:16.893998Z',
    status: 'SUSPECTFOUND',
    suspect: {
      gitilesCommit: {
        host: 'testHost',
        project: 'testProject',
        ref: 'test/ref/dev',
        id: 'commit5',
      },
      reviewUrl: 'http://this/is/review/url',
      reviewTitle: 'Review title',
      verificationDetails: {
        status: "Vindicated",
      },
    },
    reruns: [
      {
        bbid: '5555',
        startTime: '2022-09-06T07:13:16.398865Z',
        endTime: '2022-09-06T07:13:16.893998Z',
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'commit5',
        },
        rerunResult: {
          rerunStatus: 'RERUN_STATUS_FAILED',
        },
        index: "5",
        type: "NthSection",
      },
      {
        bbid: '6666',
        startTime: '2022-09-06T07:13:16.398865Z',
        endTime: '2022-09-06T07:13:16.893998Z',
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'commit6',
        },
        rerunResult: {
          rerunStatus: 'RERUN_STATUS_PASSED',
        },
        index: "6",
        type: "NthSection",
      }
    ],
    blameList: {
      commits: [],
    }
  };
};