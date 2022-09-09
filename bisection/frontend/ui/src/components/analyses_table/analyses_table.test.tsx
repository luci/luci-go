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

import { BrowserRouter } from 'react-router-dom';

import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';

import { AnalysesTable } from './analyses_table';
import { getMockAnalysis } from '../../testing_tools/mocks/analysis_mock';

describe('Test AnalysesTable component', () => {
  test('if analyses are displayed', async () => {
    const mockAnalyses = [
      getMockAnalysis('123'),
      getMockAnalysis('124'),
      getMockAnalysis('125'),
    ];

    render(
      <BrowserRouter>
        <AnalysesTable analyses={mockAnalyses} />
      </BrowserRouter>
    );

    await screen.findByText('Buildbucket ID');

    // Check there is a link displayed in the table for each analysis
    mockAnalyses.forEach((mockAnalysis) => {
      const analysisBuildLink = screen.getByText(mockAnalysis.firstFailedBbid);
      expect(analysisBuildLink).toBeInTheDocument();
      expect(analysisBuildLink.getAttribute('href')).not.toBe('');
    });
  });

  test('if an appropriate message is displayed for no analyses', async () => {
    render(
      <BrowserRouter>
        <AnalysesTable analyses={[]} />
      </BrowserRouter>
    );

    await screen.findByText('Buildbucket ID');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('No analyses to display')).toBeInTheDocument();
  });
});
