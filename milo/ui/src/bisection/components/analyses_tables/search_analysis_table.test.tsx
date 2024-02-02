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

import {
  mockErrorQueryingAnalysis,
  mockQueryAnalysis,
} from '@/bisection/testing_tools/mocks/analysis_mock';
import { createMockAnalysis } from '@/bisection/testing_tools/mocks/analysis_mock';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SearchAnalysisTable } from './search_analysis_table';

describe('<SearchAnalysisTable />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if the matching analysis is displayed', async () => {
    const mockAnalyses: Analysis[] = [createMockAnalysis('123')];
    mockQueryAnalysis(mockAnalyses);

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    // Check there is a link displayed in the table for the analysis
    mockAnalyses.forEach((mockAnalysis) => {
      const analysisBuildLink = screen.getByText(mockAnalysis.firstFailedBbid);
      expect(analysisBuildLink).toBeInTheDocument();
      expect(analysisBuildLink.getAttribute('href')).not.toBe('');
    });
  });

  test('if a related analysis is displayed', async () => {
    const mockAnalyses: Analysis[] = [createMockAnalysis('123')];
    mockQueryAnalysis(mockAnalyses);

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="124" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    // Check there is a link displayed in the table for the analysis
    mockAnalyses.forEach((mockAnalysis) => {
      const analysisBuildLink = screen.getByText(mockAnalysis.firstFailedBbid);
      expect(analysisBuildLink).toBeInTheDocument();
      expect(analysisBuildLink.getAttribute('href')).not.toBe('');
    });

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('Found related analysis')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for no analysis', async () => {
    const mockAnalyses: Analysis[] = [];
    mockQueryAnalysis(mockAnalyses);

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('No analysis found for build 123'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error', async () => {
    mockErrorQueryingAnalysis();

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    expect(screen.getByText('Issue searching by build')).toBeInTheDocument();
    expect(screen.queryByText('Buildbucket ID')).not.toBeInTheDocument();
  });
});
