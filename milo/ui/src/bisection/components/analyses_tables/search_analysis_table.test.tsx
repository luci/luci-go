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
  mockGetBuild,
  mockNoBuild,
  mockErrorQueryingBuild,
} from '@/bisection/testing_tools/mocks/analysis_mock';
import { createMockAnalysis } from '@/bisection/testing_tools/mocks/analysis_mock';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SearchAnalysisTable } from './search_analysis_table';

jest.mock('@/common/hooks/prpc_clients', () => {
  return {
    ...jest.requireActual('@/common/hooks/prpc_clients'),
    useBuildsClient: jest.fn(),
  };
});

describe('<SearchAnalysisTable />', () => {
  beforeEach(() => {
    const { useBuildsClient } = jest.requireMock('@/common/hooks/prpc_clients');
    useBuildsClient.mockReturnValue({
      GetBuild: {
        query: (req: GetBuildRequest) => {
          const { queryKey, queryFn } = jest
            .requireActual('@/common/hooks/prpc_clients')
            .useBuildsClient()
            .GetBuild.query(req);
          return { queryKey, queryFn };
        },
      },
    });
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  test('if the matching analysis is displayed', async () => {
    const mockAnalyses: Analysis[] = [createMockAnalysis('123')];
    const mockBuild: Build = Build.fromPartial({
      id: '123',
      steps: [
        {
          name: 'some-step',
          status: Status.FAILURE,
        },
      ],
    });
    mockGetBuild('123', mockBuild);
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
    const mockBuild: Build = Build.fromPartial({
      id: '124',
      steps: [
        {
          name: 'some-step',
          status: Status.FAILURE,
        },
      ],
    });
    mockGetBuild('124', mockBuild);
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
    const mockBuild: Build = Build.fromPartial({
      id: '123',
      steps: [
        {
          name: 'some-step',
          status: Status.FAILURE,
        },
      ],
    });
    mockGetBuild('123', mockBuild);
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
    const mockBuild: Build = Build.fromPartial({
      id: '123',
      steps: [
        {
          name: 'some-step',
          status: Status.FAILURE,
        },
      ],
    });
    mockGetBuild('123', mockBuild);
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

  test('if an appropriate message is displayed for a missing build', async () => {
    mockNoBuild();

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    expect(screen.getByText('Build 123 not found')).toBeInTheDocument();
    expect(screen.queryByText('Buildbucket ID')).not.toBeInTheDocument();
  });

  test('if an appropriate message is displayed for an error querying a build', async () => {
    mockErrorQueryingBuild();

    render(
      <FakeContextProvider>
        <SearchAnalysisTable bbid="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('search-analysis-table');

    expect(screen.getByText('Error fetching build 123')).toBeInTheDocument();
    expect(screen.queryByText('Buildbucket ID')).not.toBeInTheDocument();
  });
});
