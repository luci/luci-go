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
import fetchMock from 'fetch-mock-jest';
import { act } from 'react';

import {
  mockErrorFetchingAnalyses,
  mockFetchAnalyses,
} from '@/bisection/testing_tools/mocks/analyses_mock';
import { createMockAnalysis } from '@/bisection/testing_tools/mocks/analysis_mock';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ListAnalysesTable } from './list_analyses_table';

describe('<AnalysesTable />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if analyses are displayed', async () => {
    const mockAnalyses = [
      createMockAnalysis('123'),
      createMockAnalysis('124'),
      createMockAnalysis('125'),
    ];
    mockFetchAnalyses(mockAnalyses, '');

    render(
      <FakeContextProvider>
        <ListAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    // Check there is a link displayed in the table for each analysis
    mockAnalyses.forEach((mockAnalysis) => {
      const analysisBuildLink = screen.getByText(mockAnalysis.firstFailedBbid);
      expect(analysisBuildLink).toBeInTheDocument();
      expect(analysisBuildLink.getAttribute('href')).not.toBe('');
    });
  });

  test('if analyses are paginated', async () => {
    const firstPageMockAnalyses: Analysis[] = [];
    for (let i = 0; i < 25; i++) {
      firstPageMockAnalyses.push(createMockAnalysis(`${100 + i}`));
    }
    mockFetchAnalyses(firstPageMockAnalyses, 'test-token');

    render(
      <FakeContextProvider>
        <ListAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    // Check the displayed rows label is correct
    expect(await screen.findByText('1-25')).toBeInTheDocument();

    // Check the "next" button is not disabled
    const nextButton = screen.getByTitle('Go to next page');
    expect(nextButton).not.toHaveAttribute('disabled');

    // Prepare the next page of analyses
    const secondPageMockAnalyses: Analysis[] = [];
    for (let i = 25; i < 40; i++) {
      secondPageMockAnalyses.push(createMockAnalysis(`${100 + i}`));
    }
    mockFetchAnalyses(secondPageMockAnalyses, '');

    act(() => nextButton.click());
    await screen.findByText('Rows per page:');

    // Check the displayed rows label is correct
    expect(screen.getByText('26-40')).toBeInTheDocument();

    // Check the "next" button is disabled as there are no more analyses
    expect(screen.getByTitle('Go to next page')).toHaveAttribute('disabled');
  });

  test('if an appropriate message is displayed for no analyses', async () => {
    const mockAnalyses: Analysis[] = [];
    mockFetchAnalyses(mockAnalyses, '');

    render(
      <FakeContextProvider>
        <ListAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('No analyses')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error', async () => {
    mockErrorFetchingAnalyses();

    render(
      <FakeContextProvider>
        <ListAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    expect(screen.getByText('Failed to load analyses')).toBeInTheDocument();
    expect(screen.queryByText('Buildbucket ID')).not.toBeInTheDocument();
  });
});
