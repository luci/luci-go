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

import fetchMock from 'fetch-mock-jest';

import {
  fireEvent,
  screen,
} from '@testing-library/react';

import {
  QueryTestVariantFailureRateRequest,
  QueryTestVariantFailureRateResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockClusterExoneratedTestVariant,
  mockQueryExoneratedTestVariants,
} from '@/testing_tools/mocks/cluster_mock';
import {
  getMockTestVariantIdentifier,
  getMockTestVariantFailureRateAnalysis,
  mockQueryFailureRate,
} from '@/testing_tools/mocks/test_variants_mock';


import { ClusterContextProvider } from '../../cluster_context';
import ExonerationsTable from './exonerations_tab';

describe('Test ExonerationsTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
    const mockTestVariants = [
      getMockClusterExoneratedTestVariant('someTestIdAOne', 101),
      getMockClusterExoneratedTestVariant('someTestIdCThree', 303),
      getMockClusterExoneratedTestVariant('someTestIdBTwo', 202),
    ];
    mockQueryExoneratedTestVariants('projects/testproject/clusters/rules/rule-41711/exoneratedTestVariants', mockTestVariants);

    const request: QueryTestVariantFailureRateRequest = {
      project: 'testproject',
      testVariants: [
        getMockTestVariantIdentifier('someTestIdAOne'),
        getMockTestVariantIdentifier('someTestIdCThree'),
        getMockTestVariantIdentifier('someTestIdBTwo'),
      ],
    };
    const response: QueryTestVariantFailureRateResponse = {
      intervals: [], // Implementation does not read this data.
      testVariants: [
        getMockTestVariantFailureRateAnalysis('someTestIdAOne'),
        getMockTestVariantFailureRateAnalysis('someTestIdCThree'),
        getMockTestVariantFailureRateAnalysis('someTestIdBTwo'),
      ],
    };
    mockQueryFailureRate(request, response);
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given cluster failures, should group and display them', async () => {
    renderTabWithRouterAndClient(
        <ClusterContextProvider project='testproject' clusterAlgorithm='rules' clusterId='rule-41711'>
          <ExonerationsTable value='test' />
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    expect(screen.getByText('someTestIdAOne')).toBeInTheDocument();
    expect(screen.getByText('someTestIdBTwo')).toBeInTheDocument();
    expect(screen.getByText('someTestIdCThree')).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify row order', async () => {
    renderTabWithRouterAndClient(
        <ClusterContextProvider project='testproject' clusterAlgorithm='rules' clusterId='rule-41711'>
          <ExonerationsTable value='test' />
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');

    await fireEvent.click(screen.getByText('Presubmit-Blocking Failures Exonerated (7 days)'));

    const failureCountCells = screen.getAllByTestId('exonerations_table_critical_failures_cell');
    expect(failureCountCells.length).toBe(3);
    expect(failureCountCells[0]).toHaveTextContent('303');
    expect(failureCountCells[1]).toHaveTextContent('202');
    expect(failureCountCells[2]).toHaveTextContent('101');

    await fireEvent.click(screen.getByText('Test'));

    const testCells = screen.getAllByTestId('exonerations_table_test_cell');
    expect(testCells.length).toBe(3);
    expect(testCells[0]).toHaveTextContent('someTestIdCThree');
    expect(testCells[1]).toHaveTextContent('someTestIdBTwo');
    expect(testCells[2]).toHaveTextContent('someTestIdAOne');
  });
});
