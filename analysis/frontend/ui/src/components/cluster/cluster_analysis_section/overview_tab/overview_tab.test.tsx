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

import { screen } from '@testing-library/react';

import { ClusterContextProvider } from '@/components/cluster/cluster_context';
import { QueryClusterHistoryResponse } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockGetCluster,
  mockQueryHistory,
} from '@/testing_tools/mocks/cluster_mock';
import {
  getMockMetricsList,
  mockFetchMetrics,
} from '@/testing_tools/mocks/metrics_mock';
import {
  createMockProjectConfig,
  mockFetchProjectConfig,
} from '@/testing_tools/mocks/projects_mock';

import { createDefaultMockRule, mockFetchRule } from '@/testing_tools/mocks/rule_mock';
import OverviewTab from './overview_tab';

// Mock the window.ResizeObserver that is needed by recharts.
class ResizeObserver {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  observe() { }
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  unobserve() { }
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  disconnect() { }
}
window.ResizeObserver = ResizeObserver;

describe('Test OverviewTab component', () => {
  const project = 'chrome';
  const mockMetrics = getMockMetricsList(project);

  beforeEach(() => {
    mockFetchAuthState();
    mockFetchMetrics(project, mockMetrics);
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a project and cluster ID, should show problems and cluster history for that cluster', async () => {
    const algorithm = 'rules';
    const id = '123456';

    mockFetchRule(createDefaultMockRule());
    const mockCluster = getMockCluster(id, project, algorithm);
    mockGetCluster(project, algorithm, id, mockCluster);
    const mockConfig = createMockProjectConfig();
    mockFetchProjectConfig(mockConfig);

    const history: QueryClusterHistoryResponse = {
      days: [{
        date: '2023-02-16',
        metrics: {
          'human-cls-failed-presubmit': 10,
          'critical-failures-exonerated': 20,
          'test-runs-failed': 100,
        },
      }],
    };
    mockQueryHistory(history);

    renderTabWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTab value='test' />
        </ClusterContextProvider>,
    );

    await screen.findByTestId('policy-row-exonerations');
    expect(screen.getByText('test variant(s) are being exonerated (ignored) in presubmit')).toBeInTheDocument();
    expect(screen.getByText('many CL(s) are being rejected')).toBeInTheDocument();

    await screen.findByTestId('history-charts-container');
    // Expect charts only for the default metrics.
    mockMetrics.filter((metric) => metric.isDefault).forEach((metric) => {
      expect(
          screen.getByTestId('chart-' + metric.metricId),
      ).toBeInTheDocument();
    });
    mockMetrics.filter((metric) => !metric.isDefault).forEach((metric) => {
      expect(
          screen.queryByTestId('chart-' + metric.metricId),
      ).toBeNull();
    });
  });
});
