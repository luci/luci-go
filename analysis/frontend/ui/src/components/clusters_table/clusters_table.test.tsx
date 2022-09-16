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
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
} from '@/services/cluster';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockRuleClusterSummary,
  getMockSuggestedClusterSummary,
  mockQueryClusterSummaries,
} from '@/testing_tools/mocks/cluster_mock';

import ClustersTable from './clusters_table';

describe('Test ClustersTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given clusters, it should display them', async () => {
    const mockClusters = [
      getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleClusterSummary('10000000000000001000000000000000'),
    ];
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'critical_failures_exonerated desc',
      failureFilter: '',
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: mockClusters };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable
          project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    expect(screen.getByText(mockClusters[1].bug!.linkText)).toBeInTheDocument();
  });

  it('given no clusters, it should display an appropriate message', async () => {
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'critical_failures_exonerated desc',
      failureFilter: '',
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable
          project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('Hooray! There are no failures matching the specified criteria.')).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify cluster order', async () => {
    const suggestedCluster = getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'critical_failures_exonerated desc',
      failureFilter: '',
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable
          project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after sorting.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'failures desc',
      failureFilter: '',
    };
    const ruleCluster2 = getMockRuleClusterSummary('20000000000000002000000000000000');
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    ruleCluster2.bug!.linkText = 'crbug.com/2222222';
    const updatedResponse: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster2],
    };
    mockQueryClusterSummaries(updatedRequest, updatedResponse);

    await fireEvent.click(screen.getByText('Total Failures'));

    await screen.findByText('crbug.com/2222222');
    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('crbug.com/2222222')).toBeInTheDocument();
  });

  it('when filtering it should show matching failures', async () => {
    const suggestedCluster = getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'critical_failures_exonerated desc',
      failureFilter: '',
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable
          project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after filtering.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'critical_failures_exonerated desc',
      failureFilter: 'new_criteria',
    };
    const ruleCluster2 = getMockRuleClusterSummary('20000000000000002000000000000000');
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    ruleCluster2.bug!.linkText = 'crbug.com/3333333';
    const updatedResponse: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster2],
    };
    mockQueryClusterSummaries(updatedRequest, updatedResponse);

    fireEvent.change(screen.getByTestId('failure_filter_input'), { target: { value: 'new_criteria' } });
    fireEvent.blur(screen.getByTestId('failure_filter_input'));

    await screen.findByText('crbug.com/3333333');

    expect(screen.getByText('crbug.com/3333333')).toBeInTheDocument();
  });
});
