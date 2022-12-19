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

import { HeuristicAnalysisTable } from './heuristic_analysis_table';
import { createMockHeuristicSuspect } from '../../testing_tools/mocks/heuristic_suspect_mock';
import { HeuristicAnalysisResult } from '../../services/luci_bisection';

describe('Test HeuristicAnalysisTable component', () => {
  test('if an appropriate message is displayed for no analysis', async () => {
    render(<HeuristicAnalysisTable />);

    await screen.findByTestId('heuristic-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('There is no heuristic analysis')
    ).toBeInTheDocument();
  });

  test('if heuristic suspects are displayed', async () => {
    const mockSuspects = [
      createMockHeuristicSuspect('ac52e3'),
      createMockHeuristicSuspect('673e20'),
    ];

    const mockHeuristicAnalysisResult: HeuristicAnalysisResult = {
      status: 'SUSPECTFOUND',
      suspects: mockSuspects,
    };

    render(<HeuristicAnalysisTable result={mockHeuristicAnalysisResult} />);

    await screen.findByTestId('heuristic-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(mockSuspects.length);
  });

  test('if an appropriate message is displayed for no suspects', async () => {
    const mockHeuristicAnalysisResult: HeuristicAnalysisResult = {
      status: 'NOTFOUND',
      suspects: [],
    };
    render(<HeuristicAnalysisTable result={mockHeuristicAnalysisResult} />);

    await screen.findByTestId('heuristic-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('No suspects found')).toBeInTheDocument();
  });

  test('if no misleading message is shown for an incomplete analysis', async () => {
    const mockHeuristicAnalysisResult: HeuristicAnalysisResult = {
      status: 'RUNNING',
      suspects: [],
    };
    render(<HeuristicAnalysisTable result={mockHeuristicAnalysisResult} />);

    await screen.findByTestId('heuristic-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('Heuristic analysis is in progress')
    ).toBeInTheDocument();
    expect(screen.queryByText('No suspects found')).not.toBeInTheDocument();
  });
});
