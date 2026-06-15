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

import { screen } from '@testing-library/react';

import { renderTabWithRouterAndClient } from '@/clusters/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockGetCluster,
  mockQueryHistory,
} from '@/clusters/testing_tools/mocks/cluster_mock';
import {
  getMockMetricsList,
  mockFetchMetrics,
} from '@/clusters/testing_tools/mocks/metrics_mock';
import { QueryClusterHistoryResponse } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { resetMockFetch } from '@/testing_tools/jest_utils';

import { ClusterContextProvider } from './../../provider';
import OverviewTab from './overview_tab';

describe('Test OverviewTab component', () => {
  const project = 'chrome';
  const mockMetrics = getMockMetricsList(project);

  beforeEach(() => {
    mockFetchAuthState();
    mockFetchMetrics(project, mockMetrics);
  });

  afterEach(() => {
    resetMockFetch();
  });

  it('given a project and cluster ID, should show problems and cluster history for that cluster', async () => {
    const algorithm = 'rules';
    const id = '123456';

    const mockCluster = getMockCluster(id, project, algorithm);
    mockGetCluster(project, algorithm, id, mockCluster);

    const history: QueryClusterHistoryResponse = {
      days: [
        {
          date: '2023-02-16',
          metrics: {
            'human-cls-failed-presubmit': 10,
            'critical-failures-exonerated': 20,
            'test-runs-failed': 100,
          },
        },
      ],
    };
    mockQueryHistory(history);

    const defaultMetrics = mockMetrics.filter((m) => m.isDefault);
    const route = `/?selectedMetrics=${defaultMetrics
      .map((m) => m.metricId)
      .join(',')}&historyTimeRange=30d&annotated=false`;

    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <OverviewTab value="test" />
      </ClusterContextProvider>,
      'test',
      route,
    );

    // Wait for the cluster analysis overview tab to load.
    await screen.findByTestId('history-charts-container');

    // Expect charts only for the default metrics.
    mockMetrics
      .filter((metric) => metric.isDefault)
      .forEach((metric) => {
        expect(
          screen.getByTestId('chart-' + metric.metricId),
        ).toBeInTheDocument();
      });
    mockMetrics
      .filter((metric) => !metric.isDefault)
      .forEach((metric) => {
        expect(screen.queryByTestId('chart-' + metric.metricId)).toBeNull();
      });
  });
});
