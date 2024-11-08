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

import '@testing-library/jest-dom';

import { fireEvent, screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { renderTabWithRouterAndClient } from '@/clusters/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  getMockClusterExoneratedTestVariantBranch,
  mockQueryExoneratedTestVariantBranches,
} from '@/clusters/testing_tools/mocks/cluster_mock';
import {
  getMockTestVariantPosition,
  getMockTestVariantStabilityAnalysis,
  getMockTestStabilityCriteria,
  mockQueryStability,
} from '@/clusters/testing_tools/mocks/test_variants_mock';
import {
  QueryTestVariantStabilityRequest,
  QueryTestVariantStabilityResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

import { ClusterContextProvider } from '../../cluster_context';

import ExonerationsTable from './exonerations_v2_tab';

describe('Test ExonerationsTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
    const mockTestVariantBranches = [
      getMockClusterExoneratedTestVariantBranch('someTestIdAOne', 1001),
      getMockClusterExoneratedTestVariantBranch('someTestIdCThree', 3003),
      getMockClusterExoneratedTestVariantBranch('someTestIdBTwo', 2002),
    ];
    mockQueryExoneratedTestVariantBranches(
      'projects/testproject/clusters/rules/rule-41711/exoneratedTestVariantBranches',
      mockTestVariantBranches,
    );

    const request: QueryTestVariantStabilityRequest = {
      project: 'testproject',
      testVariants: [
        getMockTestVariantPosition('someTestIdAOne'),
        getMockTestVariantPosition('someTestIdCThree'),
        getMockTestVariantPosition('someTestIdBTwo'),
      ],
    };
    const response: QueryTestVariantStabilityResponse = {
      testVariants: [
        getMockTestVariantStabilityAnalysis('someTestIdAOne'),
        getMockTestVariantStabilityAnalysis('someTestIdCThree'),
        getMockTestVariantStabilityAnalysis('someTestIdBTwo'),
      ],
      criteria: getMockTestStabilityCriteria(),
    };
    mockQueryStability(request, response);
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given cluster failures, should group and display them', async () => {
    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project="testproject"
        clusterAlgorithm="rules"
        clusterId="rule-41711"
      >
        <ExonerationsTable value="test" />
      </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    expect(screen.getByText('someTestIdAOne')).toBeInTheDocument();
    expect(screen.getByText('someTestIdBTwo')).toBeInTheDocument();
    expect(screen.getByText('someTestIdCThree')).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify row order', async () => {
    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project="testproject"
        clusterAlgorithm="rules"
        clusterId="rule-41711"
      >
        <ExonerationsTable value="test" />
      </ClusterContextProvider>,
    );

    await screen.findByRole('table');

    await fireEvent.click(
      screen.getByText('Presubmit-Blocking Failures Exonerated (7 days)'),
    );

    const failureCountCells = screen.getAllByTestId(
      'exonerations_table_critical_failures_cell',
    );
    expect(failureCountCells.length).toBe(3);
    expect(failureCountCells[0]).toHaveTextContent('3003');
    expect(failureCountCells[1]).toHaveTextContent('2002');
    expect(failureCountCells[2]).toHaveTextContent('1001');

    await fireEvent.click(screen.getByText('Test'));

    const testCells = screen.getAllByTestId('exonerations_table_test_cell');
    expect(testCells.length).toBe(3);
    expect(testCells[0]).toHaveTextContent('someTestIdCThree');
    expect(testCells[1]).toHaveTextContent('someTestIdBTwo');
    expect(testCells[2]).toHaveTextContent('someTestIdAOne');
  });
});
