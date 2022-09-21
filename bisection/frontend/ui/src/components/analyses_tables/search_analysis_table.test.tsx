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

import fetchMock from 'fetch-mock-jest';

import '@testing-library/jest-dom';
import { screen } from '@testing-library/react';

import { SearchAnalysisTable } from './search_analysis_table';
import { Analysis } from '../../services/luci_bisection';
import { createMockAnalysis } from '../../testing_tools/mocks/analysis_mock';
import {
  mockErrorQueryingAnalysis,
  mockQueryAnalysis,
} from '../../testing_tools/mocks/analysis_mock';
import { renderWithRouterAndClient } from '../../testing_tools/libs/mock_router';
import { mockFetchAuthState } from '../../testing_tools/mocks/authstate_mock';

describe('Test SearchAnalysisTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if the matching analysis is displayed', async () => {
    const mockAnalyses: Analysis[] = [createMockAnalysis('123')];
    mockQueryAnalysis(mockAnalyses);

    renderWithRouterAndClient(<SearchAnalysisTable bbid='123' />);

    await screen.findByText('Buildbucket ID');

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

    renderWithRouterAndClient(<SearchAnalysisTable bbid='124' />);

    await screen.findByText('Buildbucket ID');

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

    renderWithRouterAndClient(<SearchAnalysisTable bbid='123' />);

    await screen.findByText('Buildbucket ID');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('No analysis found for build 123')
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error', async () => {
    mockErrorQueryingAnalysis();

    renderWithRouterAndClient(<SearchAnalysisTable bbid='123' />);

    await screen.findByRole('alert');

    expect(screen.getByText('Issue searching by build')).toBeInTheDocument();
    expect(screen.queryByText('Buildbucket ID')).not.toBeInTheDocument();
  });
});
