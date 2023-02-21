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
  waitFor,
  within,
} from '@testing-library/react';

import {
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
} from '@/services/cluster';
import { Metric } from '@/services/metrics';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockRuleClusterSummary,
  getMockSuggestedClusterSummary,
  mockQueryClusterSummaries,
} from '@/testing_tools/mocks/cluster_mock';
import { mockFetchMetrics } from '@/testing_tools/mocks/metrics_mock';

import ClustersTable from './clusters_table';

describe('Test ClustersTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('should display column headings reflecting the system metrics', async () => {
    const metrics : Metric[] = [{
      name: 'metrics/metric-a',
      metricId: 'metric-a',
      humanReadableName: 'Metric Alpha',
      description: 'Metric alpha is the first metric',
      isDefault: true,
      sortPriority: 20,
    }, {
      name: 'metrics/metric-b',
      metricId: 'metric-b',
      humanReadableName: 'Metric Beta',
      description: 'Metric beta is the second metric',
      isDefault: true,
      sortPriority: 30,
    }, {
      name: 'metrics/metric-c',
      metricId: 'metric-c',
      humanReadableName: 'Metric Charlie',
      description: 'Metric charlie is the third metric',
      isDefault: false,
      sortPriority: 10,
    }];
    mockFetchMetrics(metrics);

    // Only default metrics (i.e. metric A and B) should be queried and shown.
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`metric-b`.value desc',
      failureFilter: '',
      metrics: ['metrics/metric-a', 'metrics/metric-b'],
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject"/>,
    );
    await screen.findByTestId('clusters_table_body');
    expect(screen.getByText('Metric Alpha')).toBeInTheDocument();
    expect(screen.getByText('Metric Beta')).toBeInTheDocument();
  });

  it('given clusters, it should display them', async () => {
    mockFetchMetrics();

    const mockClusters = [
      getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleClusterSummary('10000000000000001000000000000000'),
    ];
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: mockClusters };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    await waitFor(() => expect(screen.getByText(mockClusters[1].bug!.linkText)).toBeInTheDocument());
  });

  it('given no clusters, it should display an appropriate message', async () => {
    mockFetchMetrics();

    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('Hooray! There are no failures matching the specified criteria.')).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify cluster order', async () => {
    mockFetchMetrics();

    const suggestedCluster = getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after sorting.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`failures`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
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
    mockFetchMetrics();

    const suggestedCluster = getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject"/>,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after filtering.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: 'new_criteria',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
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

    await waitFor(() => expect(screen.getByText('crbug.com/3333333')).toBeInTheDocument());
  });

  it('when changing metrics should hide columns', async () => {
    mockFetchMetrics();

    const mockClusters = [
      getMockSuggestedClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleClusterSummary('10000000000000001000000000000000'),
    ];
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['metrics/human-cls-failed-presubmit',
        'metrics/critical-failures-exonerated',
        'metrics/failures'],
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: mockClusters };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_head');

    expect(screen.getByText('User Cls Failed Presubmit')).toBeInTheDocument();

    fireEvent.mouseDown(within(screen.getByTestId('metrics-selection')).getByRole('button'));

    const request2 = {
      project: 'testproject',
      failureFilter: '',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      metrics: ['metrics/critical-failures-exonerated', 'metrics/failures'],
    };
    const response2 = { clusterSummaries: mockClusters };

    mockQueryClusterSummaries(request2, response2);

    const listOfItems = within(screen.getByRole('listbox'));
    fireEvent.click(listOfItems.getByText('User Cls Failed Presubmit'));

    await waitFor(() => {
      expect(screen.getByTestId('clusters_table_head')).toBeInTheDocument();
      expect(within(screen.getByTestId('clusters_table_head')).queryByText('User Cls Failed Presubmit'))
          .not.toBeInTheDocument();
    });
  });

  it('when removing order by column, should select highest sort order in selected metrics', async () => {
    const metrics : Metric[] = [{
      name: 'metrics/metric-a',
      metricId: 'metric-a',
      humanReadableName: 'Metric Alpha',
      description: 'Metric alpha is the first metric',
      isDefault: true,
      sortPriority: 20,
    }, {
      name: 'metrics/metric-b',
      metricId: 'metric-b',
      humanReadableName: 'Metric Beta',
      description: 'Metric beta is the second metric',
      isDefault: true,
      sortPriority: 10,
    }, {
      name: 'metrics/metric-c',
      metricId: 'metric-c',
      humanReadableName: 'Metric Charlie',
      description: 'Metric charlie is the third metric',
      isDefault: false,
      sortPriority: 30,
    }];
    mockFetchMetrics(metrics);

    // Only default metrics (i.e. metric A and B) should be queried and shown.
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      orderBy: 'metrics.`metric-a`.value desc',
      failureFilter: '',
      metrics: ['metrics/metric-a', 'metrics/metric-b'],
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject"/>,
    );
    await screen.findByTestId('clusters_table_body');
    expect(screen.getByText('Metric Alpha')).toBeInTheDocument();
    expect(screen.getByText('Metric Beta')).toBeInTheDocument();

    fireEvent.mouseDown(within(screen.getByTestId('metrics-selection')).getByRole('button'));

    const request2: QueryClusterSummariesRequest = { 'project': 'testproject',
      'failureFilter': '',
      'orderBy': 'metrics.`metric-a`.value desc',
      'metrics': ['metrics/metric-a', 'metrics/metric-b', 'metrics/metric-c'],
    };
    const response2 = { clusterSummaries: [] };

    mockQueryClusterSummaries(request2, response2);

    const listOfItems = within(screen.getByRole('listbox'));
    await fireEvent.click(listOfItems.getByText('Metric Charlie'));
    await screen.findByTestId('clusters_table_head');

    const request3 = {
      project: 'testproject',
      failureFilter: '',
      orderBy: 'metrics.`metric-c`.value desc',
      metrics: ['metrics/metric-b', 'metrics/metric-c'],
    };
    const response3 = { clusterSummaries: [] };

    mockQueryClusterSummaries(request3, response3);
    await fireEvent.click(listOfItems.getByText('Metric Alpha'));
    await screen.findByTestId('clusters_table_head');

    await waitFor(() => {
      expect(screen.getByTestId('clusters_table_head')).toBeInTheDocument();
      expect(within(screen.getByTestId('clusters_table_head')).getByText('Metric Charlie')).toBeInTheDocument();
      expect(within(screen.getByTestId('clusters_table_head')).queryByText('Metric Alpha')).not.toBeInTheDocument();
    });
  });
});
