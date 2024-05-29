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
  createMockTestAnalysis,
  mockErrorListTestAnalyses,
  mockListTestAnalyses,
} from '@/bisection/testing_tools/mocks/test_analysis_mock';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ListTestAnalysesTable } from './list_test_analyses_table';

describe('<AnalysesTable />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if analyses are displayed', async () => {
    const mockAnalyses = [
      createMockTestAnalysis('123'),
      createMockTestAnalysis('124'),
      createMockTestAnalysis('125'),
    ];
    mockListTestAnalyses(mockAnalyses, '');

    render(
      <FakeContextProvider
        mountedPath="/p/:project/bisection/test_analysis"
        routerOptions={{
          initialEntries: ['/p/testproject/bisection/test_analysis'],
        }}
      >
        <ListTestAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    // Check there is a link displayed in the table for each analysis
    mockAnalyses.forEach((mockAnalysis) => {
      const analysisIDLink = screen.getByText(mockAnalysis.analysisId);
      expect(analysisIDLink).toBeInTheDocument();
      expect(analysisIDLink.getAttribute('href')).toBe(
        `/ui/bisection/test-analysis/b/${mockAnalysis.analysisId}`,
      );
    });
  });

  test('if analyses are paginated', async () => {
    const firstPageMockAnalyses: TestAnalysis[] = [];
    for (let i = 0; i < 25; i++) {
      firstPageMockAnalyses.push(createMockTestAnalysis(`${100 + i}`));
    }
    mockListTestAnalyses(firstPageMockAnalyses, 'test-token');

    render(
      <FakeContextProvider
        mountedPath="/p/:project/bisection/test_analysis"
        routerOptions={{
          initialEntries: ['/p/testproject/bisection/test_analysis'],
        }}
      >
        <ListTestAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    // Check the displayed rows label is correct
    expect(await screen.findByText('1-25')).toBeInTheDocument();

    // Check the "next" button is not disabled
    const nextButton = screen.getByTitle('Go to next page');
    expect(nextButton).not.toHaveAttribute('disabled');

    // Prepare the next page of analyses
    const secondPageMockAnalyses: TestAnalysis[] = [];
    for (let i = 25; i < 40; i++) {
      secondPageMockAnalyses.push(createMockTestAnalysis(`${100 + i}`));
    }
    mockListTestAnalyses(secondPageMockAnalyses, '');

    act(() => nextButton.click());
    await screen.findByText('Rows per page:');

    // Check the displayed rows label is correct
    expect(screen.getByText('26-40')).toBeInTheDocument();

    // Check the "next" button is disabled as there are no more analyses
    expect(screen.getByTitle('Go to next page')).toHaveAttribute('disabled');
  });

  test('if an appropriate message is displayed for no analyses', async () => {
    const mockAnalyses: TestAnalysis[] = [];
    mockListTestAnalyses(mockAnalyses, '');

    render(
      <FakeContextProvider
        mountedPath="/p/:project/bisection/test_analysis"
        routerOptions={{
          initialEntries: ['/p/testproject/bisection/test_analysis'],
        }}
      >
        <ListTestAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('No analyses')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error', async () => {
    mockErrorListTestAnalyses();

    render(
      <FakeContextProvider
        mountedPath="/p/:project/bisection/test_analysis"
        routerOptions={{
          initialEntries: ['/p/testproject/bisection/test_analysis'],
        }}
      >
        <ListTestAnalysesTable />
      </FakeContextProvider>,
    );

    await screen.findByTestId('list-analyses-table');

    expect(screen.getByText('Failed to load analyses')).toBeInTheDocument();
    expect(screen.queryByText('Analysis ID')).not.toBeInTheDocument();
  });
});
