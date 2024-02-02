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

import { RpcCode } from '@chopsui/prpc-client';
import { render, screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import {
  createMockTestAnalysis,
  mockErrorGetTestAnalysis,
  mockGetTestAnalysis,
} from '@/bisection/testing_tools/mocks/test_analysis_mock';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SearchTestAnalysisTable } from './search_test_analysis_table';

describe('<SearchTestAnalysisTable />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if the matching analysis is displayed', async () => {
    const mockAnalysis: TestAnalysis = createMockTestAnalysis('123');
    mockGetTestAnalysis(mockAnalysis);

    render(
      <FakeContextProvider>
        <SearchTestAnalysisTable analysisId="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    // Check there is a link displayed in the table.
    const analysisIDLink = screen.getByText(mockAnalysis.analysisId);
    expect(analysisIDLink).toBeInTheDocument();
    expect(analysisIDLink.getAttribute('href')).toBe(
      `/ui/bisection/test-analysis/b/${mockAnalysis.analysisId}`,
    );
  });

  test('if an appropriate message is displayed for no analysis', async () => {
    mockErrorGetTestAnalysis(RpcCode.NOT_FOUND);

    render(
      <FakeContextProvider>
        <SearchTestAnalysisTable analysisId="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('No analysis found for ID 123'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error', async () => {
    mockErrorGetTestAnalysis(RpcCode.PERMISSION_DENIED);

    render(
      <FakeContextProvider>
        <SearchTestAnalysisTable analysisId="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');
    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(screen.getByText('Issue searching by ID')).toBeInTheDocument();
  });
});
