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

import { CulpritVerificationTable } from './culprit_verification_table';

import {
  Analysis,
} from '../../services/luci_bisection';


describe('Test CulpritVerificationTable component', () => {
  test('if all information is displayed', async () => {
    const mockAnalysis = createMockAnalysis()

    render(<CulpritVerificationTable result={mockAnalysis} />);

    await screen.findByText('Suspect CL');
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Verification Status')).toBeInTheDocument();
    expect(screen.getByText('Reruns')).toBeInTheDocument();
    expect(screen.getByText('def456d: CL 1')).toBeInTheDocument();
    expect(screen.getByText('Heuristic')).toBeInTheDocument();
    expect(screen.getByText('Confirmed Culprit')).toBeInTheDocument();
    expect(screen.getByText('Failed')).toHaveAttribute('href', 'https://ci.chromium.org/b/8877665544332211')
    expect(screen.getByText('Passed')).toHaveAttribute('href', 'https://ci.chromium.org/b/8765432187654321');
  });
});

function createMockAnalysis(): Analysis {
  return {
    analysisId: "1234",
    status: 'FOUND',
    lastPassedBbid: '0',
    firstFailedBbid: "1234",
    createdTime: '2022-09-06T07:13:16.398865Z',
    lastUpdatedTime: '2022-09-06T07:13:16.893998Z',
    endTime: '2022-09-06T07:13:16.893998Z',
    builder: {
      project: 'chromium/test',
      bucket: 'ci',
      builder: 'mock-builder-cc64',
    },
    buildFailureType: 'COMPILE',
    heuristicResult: {
      status: 'FOUND',
      suspects: [
        {
          gitilesCommit: {
            host: 'testHost',
            project: 'testProject',
            ref: 'test/ref/dev',
            id: 'def456def456',
            position: '298',
          },
          reviewTitle: 'CL 1',
          reviewUrl: 'https://chromium-review.googlesource.com/placeholder/+/92345',
          verificationDetails: {
            status: 'Confirmed Culprit',
            suspectRerun: {
              startTime: '2022-09-06T07:13:16.398865Z',
              endTime: '2022-09-06T07:13:18.398865Z',
              bbid: '8877665544332211',
              rerunResult: {
                rerunStatus: 'RERUN_STATUS_FAILED',
              },
              commit: {
                host: 'testHost',
                project: 'testProject',
                ref: 'test/ref/dev',
                id: 'def456def456',
              },
              type: "Culprit Verification",
            },
            parentRerun: {
              startTime: '2022-09-06T07:16:16.398865Z',
              endTime: '2022-09-06T07:16:31.398865Z',
              bbid: '8765432187654321',
              rerunResult: {
                rerunStatus: 'RERUN_STATUS_PASSED',
              },
              commit: {
                host: 'testHost',
                project: 'testProject',
                ref: 'test/ref/dev',
                id: 'def456def456',
              },
              type: "Culprit Verification",
            },
          },
          score: '10',
          justification: 'Justification',
          confidenceLevel: 'HIGH',
        },
      ]
    },
  };
};