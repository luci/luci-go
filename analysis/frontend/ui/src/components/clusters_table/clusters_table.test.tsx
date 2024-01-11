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

import dayjs from 'dayjs';
import fetchMock from 'fetch-mock-jest';
import MockDate from 'mockdate';

import {
  fireEvent,
  screen,
  waitFor,
  within,
} from '@testing-library/react';

import {
  ClusterSummaryView,
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { TimeRange } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { Metric } from '@/legacy_services/metrics';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockRuleBasicClusterSummary,
  getMockRuleFullClusterSummary,
  getMockSuggestedBasicClusterSummary,
  getMockSuggestedFullClusterSummary,
  mockQueryClusterSummaries,
} from '@/testing_tools/mocks/cluster_mock';
import { mockFetchMetrics } from '@/testing_tools/mocks/metrics_mock';

import ClustersTable from './clusters_table';

describe('Test ClustersTable component', () => {
  const testNow = '2020-01-08 11:02:03.456+10:00';

  beforeAll(() => {
    MockDate.set(testNow);
  });
  beforeEach(() => {
    mockFetchAuthState();
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });
  afterAll(() => {
    MockDate.reset();
  });

  const last24Hours: TimeRange = {
    earliest: dayjs(testNow).subtract(24, 'hours').toISOString(),
    latest: dayjs(testNow).toISOString(),
  };

  it('should display column headings reflecting the system metrics', async () => {
    const metrics: Metric[] = [{
      name: 'projects/testproject/metrics/metric-a',
      metricId: 'metric-a',
      humanReadableName: 'Metric Alpha',
      description: 'Metric alpha is the first metric',
      isDefault: true,
      sortPriority: 20,
    }, {
      name: 'projects/testproject/metrics/metric-b',
      metricId: 'metric-b',
      humanReadableName: 'Metric Beta',
      description: 'Metric beta is the second metric',
      isDefault: true,
      sortPriority: 30,
    }, {
      name: 'projects/testproject/metrics/metric-c',
      metricId: 'metric-c',
      humanReadableName: 'Metric Charlie',
      description: 'Metric charlie is the third metric',
      isDefault: false,
      sortPriority: 10,
    }];
    mockFetchMetrics('testproject', metrics);

    // Only default metrics (i.e. metric A and B) should be queried and shown.
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`metric-b`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/metric-a', 'projects/testproject/metrics/metric-b'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );
    await screen.findByTestId('clusters_table_body');
    expect(screen.getByText('Metric Alpha')).toBeInTheDocument();
    expect(screen.getByText('Metric Beta')).toBeInTheDocument();
  });

  it('given clusters, it should display them', async () => {
    mockFetchMetrics();

    const mockClusters = [
      getMockSuggestedBasicClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleBasicClusterSummary('10000000000000001000000000000000'),
    ];
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
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
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('Hooray! There are no failures matching the specified criteria.')).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify cluster order', async () => {
    mockFetchMetrics();

    const suggestedCluster = getMockSuggestedBasicClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleBasicClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after sorting.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`failures`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const ruleCluster2 = getMockRuleBasicClusterSummary('20000000000000002000000000000000');
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    ruleCluster2.bug!.linkText = 'crbug.com/2222222';
    const updatedResponse: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster2],
    };
    mockQueryClusterSummaries(updatedRequest, updatedResponse);

    fireEvent.click(screen.getByText('Total Failures'));

    await screen.findByText('crbug.com/2222222');
    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('crbug.com/2222222')).toBeInTheDocument();
  });

  it('when filtering it should show matching failures', async () => {
    mockFetchMetrics();

    const suggestedCluster = getMockSuggestedBasicClusterSummary('1234567890abcedf1234567890abcedf');
    const ruleCluster = getMockRuleBasicClusterSummary('10000000000000001000000000000000');
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = {
      clusterSummaries: [suggestedCluster, ruleCluster],
    };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_body');

    // Prepare an updated set of clusters to show after filtering.
    const updatedRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: 'new_criteria',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const ruleCluster2 = getMockRuleBasicClusterSummary('20000000000000002000000000000000');
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
      getMockSuggestedBasicClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleBasicClusterSummary('10000000000000001000000000000000'),
    ];
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: mockClusters };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_head');

    await waitFor(() => expect(screen.getByText('User Cls Failed Presubmit')).toBeInTheDocument());

    fireEvent.mouseDown(within(screen.getByTestId('metrics-selection')).getByRole('button'));

    const request2: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      failureFilter: '',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      metrics: ['projects/testproject/metrics/critical-failures-exonerated', 'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.BASIC,
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
    const metrics: Metric[] = [{
      name: 'projects/testproject/metrics/metric-a',
      metricId: 'metric-a',
      humanReadableName: 'Metric Alpha',
      description: 'Metric alpha is the first metric',
      isDefault: true,
      sortPriority: 20,
    }, {
      name: 'projects/testproject/metrics/metric-b',
      metricId: 'metric-b',
      humanReadableName: 'Metric Beta',
      description: 'Metric beta is the second metric',
      isDefault: true,
      sortPriority: 10,
    }, {
      name: 'projects/testproject/metrics/metric-c',
      metricId: 'metric-c',
      humanReadableName: 'Metric Charlie',
      description: 'Metric charlie is the third metric',
      isDefault: false,
      sortPriority: 30,
    }];
    mockFetchMetrics('testproject', metrics);

    // Only default metrics (i.e. metric A and B) should be queried and shown.
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      orderBy: 'metrics.`metric-a`.value desc',
      failureFilter: '',
      metrics: ['projects/testproject/metrics/metric-a', 'projects/testproject/metrics/metric-b'],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );
    await screen.findByTestId('clusters_table_body');
    expect(screen.getByText('Metric Alpha')).toBeInTheDocument();
    expect(screen.getByText('Metric Beta')).toBeInTheDocument();

    fireEvent.mouseDown(within(screen.getByTestId('metrics-selection')).getByRole('button'));

    const request2: QueryClusterSummariesRequest = {
      project: 'testproject',
      failureFilter: '',
      timeRange: last24Hours,
      orderBy: 'metrics.`metric-a`.value desc',
      metrics: ['projects/testproject/metrics/metric-a', 'projects/testproject/metrics/metric-b', 'projects/testproject/metrics/metric-c'],
      view: ClusterSummaryView.BASIC,
    };
    const response2 = { clusterSummaries: [] };

    mockQueryClusterSummaries(request2, response2);

    const listOfItems = within(screen.getByRole('listbox'));
    fireEvent.click(listOfItems.getByText('Metric Charlie'));
    await screen.findByTestId('clusters_table_head');

    const request3: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      failureFilter: '',
      orderBy: 'metrics.`metric-c`.value desc',
      metrics: ['projects/testproject/metrics/metric-b', 'projects/testproject/metrics/metric-c'],
      view: ClusterSummaryView.BASIC,
    };
    const response3 = { clusterSummaries: [] };

    mockQueryClusterSummaries(request3, response3);
    fireEvent.click(listOfItems.getByText('Metric Alpha'));
    await screen.findByTestId('clusters_table_head');

    await waitFor(() => {
      expect(screen.getByTestId('clusters_table_head')).toBeInTheDocument();
      expect(within(screen.getByTestId('clusters_table_head')).getByText('Metric Charlie')).toBeInTheDocument();
      expect(within(screen.getByTestId('clusters_table_head')).queryByText('Metric Alpha')).not.toBeInTheDocument();
    });
  });

  it('queries for clusters based on the selected time interval', async () => {
    mockFetchMetrics();

    // Default time interval should be last 24 hours.
    const request: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: last24Hours,
      failureFilter: '',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      metrics: [
        'projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures',
      ],
      view: ClusterSummaryView.BASIC,
    };
    const response: QueryClusterSummariesResponse = { clusterSummaries: [] };
    mockQueryClusterSummaries(request, response);

    renderWithRouterAndClient(
        <ClustersTable project="testproject" />,
    );

    await screen.findByTestId('clusters_table_body');

    expect(screen.getByText('Last 24 hours')).toBeInTheDocument();

    // Mock the parallel calls to QueryClusterSummaries for both
    // the basic and full views of cluster summaries for the last week.
    const daysInWeek = 7;
    const lastWeek: TimeRange = {
      earliest: dayjs(testNow).subtract(24 * daysInWeek, 'hours').toISOString(),
      latest: dayjs(testNow).toISOString(),
    };
    const basicSummariesRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: lastWeek,
      failureFilter: '',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      metrics: [
        'projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures',
      ],
      view: ClusterSummaryView.BASIC,
    };
    const mockBasicClusterSummaries = [
      getMockSuggestedBasicClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleBasicClusterSummary('10000000000000001000000000000000'),
    ];
    const basicSummariesResponse: QueryClusterSummariesResponse = {
      clusterSummaries: mockBasicClusterSummaries,
    };
    const fullSummariesRequest: QueryClusterSummariesRequest = {
      project: 'testproject',
      timeRange: lastWeek,
      failureFilter: '',
      orderBy: 'metrics.`critical-failures-exonerated`.value desc',
      metrics: ['projects/testproject/metrics/human-cls-failed-presubmit',
        'projects/testproject/metrics/critical-failures-exonerated',
        'projects/testproject/metrics/failures'],
      view: ClusterSummaryView.FULL,
    };
    const mockFullClusterSummaries = [
      getMockSuggestedFullClusterSummary('1234567890abcedf1234567890abcedf'),
      getMockRuleFullClusterSummary('10000000000000001000000000000000'),
    ];
    const fullSummariesResponse: QueryClusterSummariesResponse = {
      clusterSummaries: mockFullClusterSummaries,
    };
    // We need both responses, so these mocks use overwriteRoutes = false.
    mockQueryClusterSummaries(basicSummariesRequest, basicSummariesResponse, false);
    mockQueryClusterSummaries(fullSummariesRequest, fullSummariesResponse, false);

    // Change interval to the last week.
    fireEvent.mouseDown(within(screen.getByTestId('interval-selection')).getByRole('button'));
    const options = within(screen.getByRole('listbox'));
    fireEvent.click(options.getByText('Last 7 days'));

    await screen.findByTestId('clusters_table_body');

    // Check the time interval has been changed.
    expect(screen.queryByText('Last 24 hours')).not.toBeInTheDocument();
    expect(screen.getByText('Last 7 days')).toBeInTheDocument();

    // Clusters for the last week should be displayed.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    expect(screen.getByText(mockFullClusterSummaries[1].bug!.linkText)).toBeInTheDocument();

    // Wait until there are no placeholder sparklines.
    await expect(screen.queryAllByTestId('clusters_table_sparkline_skeleton')).toHaveLength(0);

    // Check there are sparklines for each metric for each cluster.
    expect(screen.queryAllByTestId('clusters_table_sparkline')).toHaveLength(6);
  });
});
