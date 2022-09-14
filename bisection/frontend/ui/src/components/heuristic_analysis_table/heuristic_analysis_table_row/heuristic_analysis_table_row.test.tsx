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

import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';

import { HeuristicAnalysisTableRow } from './heuristic_analysis_table_row';
import { HeuristicSuspect } from '../../../services/luci_bisection';
import { createMockHeuristicSuspect } from '../../../testing_tools/mocks/heuristic_suspect_mock';

describe('Test HeuristicAnalysisTable component', () => {
  test('if the details for a heuristic suspect are displayed', async () => {
    const mockSuspect: HeuristicSuspect = createMockHeuristicSuspect('ac52e3');

    render(
      <Table>
        <TableBody>
          <HeuristicAnalysisTableRow suspect={mockSuspect} />
        </TableBody>
      </Table>
    );

    await screen.findByTestId('heuristic_analysis_table_row');

    // Check there is a link to the suspect's code review
    const suspectReviewLink = screen.getByRole('link');
    expect(suspectReviewLink).toBeInTheDocument();
    expect(suspectReviewLink.getAttribute('href')).toBe(mockSuspect.reviewUrl);
    if (mockSuspect.reviewTitle) {
      expect(suspectReviewLink.textContent).toContain(mockSuspect.reviewTitle);
    }

    // Check confidence level, score and reasons are displayed
    expect(screen.getByText(mockSuspect.confidenceLevel)).toBeInTheDocument();
    expect(screen.getByText(mockSuspect.score)).toBeInTheDocument();
    const reasons = mockSuspect.justification.split('\n');
    reasons.forEach((reason) => {
      expect(screen.getByText(reason)).toBeInTheDocument();
    });
  });
});
