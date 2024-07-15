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

import { render, screen } from '@testing-library/react';

import { GenericSuspect } from '@/bisection/types';
import { RerunStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { HeuristicSuspect } from '@/proto/go.chromium.org/luci/bisection/proto/v1/heuristic.pb';
import { NthSectionSuspect } from '@/proto/go.chromium.org/luci/bisection/proto/v1/nthsection.pb';

import { CulpritVerificationTable } from './culprit_verification_table';

describe('<CulpritVerificationTable />', () => {
  test('if all information is displayed', async () => {
    const suspects = createMockSuspects();

    render(<CulpritVerificationTable suspects={suspects} />);

    await screen.findByText('Suspect CL');
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Verification Status')).toBeInTheDocument();
    expect(screen.getByText('Reruns')).toBeInTheDocument();
    expect(screen.getByText('def456d: CL 1')).toBeInTheDocument();
    expect(screen.getByText('Heuristic')).toBeInTheDocument();
    expect(screen.getByText('Confirmed Culprit')).toBeInTheDocument();
    expect(screen.getByText('Failed')).toHaveAttribute(
      'href',
      'https://ci.chromium.org/b/8877665544332211',
    );
    expect(screen.getByText('Passed')).toHaveAttribute(
      'href',
      'https://ci.chromium.org/b/8765432187654321',
    );
    expect(screen.getByText('def4def: CL 2')).toBeInTheDocument();
    expect(screen.getByText('NthSection')).toBeInTheDocument();
    expect(screen.getByText('Error')).toBeInTheDocument();
    expect(screen.getByText('Infra failed')).toHaveAttribute(
      'href',
      'https://ci.chromium.org/b/8877665544332216',
    );
    expect(screen.getByText('Canceled')).toHaveAttribute(
      'href',
      'https://ci.chromium.org/b/8765432187654327',
    );
  });
});

function createMockSuspects(): GenericSuspect[] {
  return [
    GenericSuspect.fromHeuristic(
      HeuristicSuspect.fromPartial({
        gitilesCommit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'def456def456',
          position: 298,
        },
        reviewTitle: 'CL 1',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/92345',
        verificationDetails: {
          status: 'Confirmed Culprit',
          suspectRerun: {
            startTime: '2022-09-06T07:13:16.398865Z',
            endTime: '2022-09-06T07:13:18.398865Z',
            bbid: '8877665544332211',
            rerunResult: {
              rerunStatus: RerunStatus.RERUN_STATUS_FAILED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def456',
            },
            type: 'Culprit Verification',
          },
          parentRerun: {
            startTime: '2022-09-06T07:16:16.398865Z',
            endTime: '2022-09-06T07:16:31.398865Z',
            bbid: '8765432187654321',
            rerunResult: {
              rerunStatus: RerunStatus.RERUN_STATUS_PASSED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def456',
            },
            type: 'Culprit Verification',
          },
        },
      }),
    ),
    GenericSuspect.fromNthSection(
      NthSectionSuspect.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'def4def457',
          position: 298,
        },
        reviewTitle: 'CL 2',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/99999',
        verificationDetails: {
          status: 'Error',
          suspectRerun: {
            startTime: '2022-09-06T07:13:16.398865Z',
            endTime: '2022-09-06T07:13:18.398865Z',
            bbid: '8877665544332216',
            rerunResult: {
              rerunStatus: RerunStatus.RERUN_STATUS_INFRA_FAILED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def456',
            },
            type: 'Culprit Verification',
          },
          parentRerun: {
            startTime: '2022-09-06T07:16:16.398865Z',
            endTime: '2022-09-06T07:16:31.398865Z',
            bbid: '8765432187654327',
            rerunResult: {
              rerunStatus: RerunStatus.RERUN_STATUS_CANCELED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def456',
            },
            type: 'Culprit Verification',
          },
        },
      }),
    ),
  ];
}
