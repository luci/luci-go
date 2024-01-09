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

import { render, screen } from '@testing-library/react';

import { GitilesCommit } from '@/monitoring/util/server_json';

import { LuciBisectionResultSection } from './luci_bisection_result';

it('is hidden if no result', async () => {
  render(<LuciBisectionResultSection result={null} />);
  expect(screen.queryByText('LUCI')).toBeNull();
});

it('reports not supported', async () => {
  render(<LuciBisectionResultSection result={{ is_supported: false }} />);
  expect(
    screen.getByText(
      'LUCI Bisection does not support the failure in this builder.',
    ),
  ).toBeInTheDocument();
});

it('displays a culprit', async () => {
  render(
    <LuciBisectionResultSection
      result={{
        is_supported: true,
        failed_bbid: '1234',
        analysis: {
          analysis_id: '1234',
          culprits: [{ review_url: 'url', review_title: 'Culprit CL' }],
        },
      }}
    />,
  );
  expect(screen.getByText('Culprit CL')).toBeInTheDocument();
});

it('displays an nth section suspect', async () => {
  render(
    <LuciBisectionResultSection
      result={{
        is_supported: true,
        failed_bbid: '1234',
        analysis: {
          analysis_id: '1234',
          nth_section_result: {
            suspect: { reviewTitle: 'Suspect CL', reviewUrl: 'url' },
          },
        },
      }}
    />,
  );
  expect(screen.getByText('Suspect CL')).toBeInTheDocument();
});

it('displays an nth section in progress', async () => {
  render(
    <LuciBisectionResultSection
      result={{
        is_supported: true,
        failed_bbid: '1234',
        analysis: {
          analysis_id: '1234',
          nth_section_result: {
            remaining_nth_section_range: {
              first_failed: { ...exampleCommit, id: '87654321000' },
              last_passed: { ...exampleCommit, id: '12345678000' },
            },
          },
        },
      }}
    />,
  );
  expect(screen.getByText('1234567:8765432')).toBeInTheDocument();
});

it('displays an heuristic suspect', async () => {
  render(
    <LuciBisectionResultSection
      result={{
        is_supported: true,
        failed_bbid: '1234',
        analysis: {
          analysis_id: '1234',
          heuristic_result: {
            suspects: [
              {
                reviewUrl: 'https://crrev.com/c/1234',
                confidence_level: 3,
                justification: 'Because I said so.',
                score: 0,
              },
            ],
          },
        },
      }}
    />,
  );
  expect(screen.getByText('https://crrev.com/c/1234')).toBeInTheDocument();
});

const exampleCommit: GitilesCommit = {
  host: 'host',
  project: 'project',
  ref: 'main',
  id: '1111111111111111',
};
