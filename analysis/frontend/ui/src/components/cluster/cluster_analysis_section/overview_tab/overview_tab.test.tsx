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

import { QueryClusterHistoryResponse } from '@/services/cluster';
import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { mockQueryHistory } from '@/testing_tools/mocks/cluster_mock';
import { mockFetchMetrics, getMockMetricsList } from '@/testing_tools/mocks/metrics_mock';

import { ClusterContextProvider } from '../../cluster_context';
import OverviewTab from './overview_tab';

// Mock the window.ResizeObserver that is needed by recharts.
class ResizeObserver {
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  observe() {}
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  unobserve() {}
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  disconnect() {}
}
window.ResizeObserver = ResizeObserver;

describe('test ImpactSection component', () => {
  beforeEach(() => {
    mockFetchAuthState();
    mockFetchMetrics();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a project and cluster ID, should fetch cluster history for that cluster', async () => {
    const history: QueryClusterHistoryResponse = { days: [{ date: '2023-02-16', metrics: { 'human-cls-failed-presubmit': 10, 'critical-failures-exonerated': 20, 'test-runs-failed': 100 } }] };
    mockQueryHistory(history);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='123456'>
          <OverviewTab value='test' />
        </ClusterContextProvider>,
    );

    await screen.findAllByTestId('history-chart');

    expect(screen.getByTestId('history-chart')).toBeInTheDocument();

    // Expect charts only for the default metrics.
    const metrics = getMockMetricsList();
    metrics.forEach((m) => {
      if (m.isDefault) {
        expect(screen.getByTestId('chart-' + m.metricId)).toBeInTheDocument();
      } else {
        expect(screen.queryByTestId('chart-' + m.metricId)).toBeNull();
      }
    })
  });
});
