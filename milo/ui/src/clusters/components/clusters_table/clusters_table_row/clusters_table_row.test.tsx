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

import { ClusterTableContextWrapper } from '@/clusters/components/clusters_table/clusters_table_context';
import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import {
  getMockRuleBasicClusterSummary,
  getMockRuleFullClusterSummary,
  getMockSuggestedBasicClusterSummary,
} from '@/clusters/testing_tools/mocks/cluster_mock';
import { getMockMetricsList } from '@/clusters/testing_tools/mocks/metrics_mock';

import ClustersTableRow from './clusters_table_row';

describe('Test ClustersTableRow component', () => {
  it('given a rule cluster', async () => {
    const metrics = getMockMetricsList('testproject');
    const mockCluster = getMockRuleBasicClusterSummary(
      'abcdef1234567890abcdef1234567890',
    );
    renderWithRouterAndClient(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <tbody>
            <ClustersTableRow project="testproject" cluster={mockCluster} />
          </tbody>
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,test-runs-failed,failures',
      '/',
    );

    await screen.findByText(mockCluster.title);

    expect(
      screen.getByText(mockCluster.bug?.linkText || ''),
    ).toBeInTheDocument();
    for (let i = 0; i < metrics.length; i++) {
      const metricValues = mockCluster?.metrics || {};
      const metricValue = metricValues[metrics[i].metricId] || { value: '' };
      expect(screen.getByText(metricValue.value || '0')).toBeInTheDocument();
    }
  });

  it('given a suggested cluster', async () => {
    const metrics = getMockMetricsList('testproject');
    const mockCluster = getMockSuggestedBasicClusterSummary(
      'abcdef1234567890abcdef1234567890',
    );
    renderWithRouterAndClient(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <tbody>
            <ClustersTableRow project="testproject" cluster={mockCluster} />
          </tbody>
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,test-runs-failed,failures',
      '/',
    );

    await screen.findByText(mockCluster.title);

    for (let i = 0; i < metrics.length; i++) {
      const metricValues = mockCluster?.metrics || {};
      const metricValue = metricValues[metrics[i].metricId] || { value: '' };
      expect(screen.getByText(metricValue.value || '0')).toBeInTheDocument();
    }
  });

  it('shows placeholders for sparklines when breakdown is loading', async () => {
    const metrics = getMockMetricsList('testproject');
    const mockCluster = getMockRuleBasicClusterSummary(
      'abcdef1234567890abcdef1234567890',
    );
    renderWithRouterAndClient(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <tbody>
            <ClustersTableRow
              project="testproject"
              cluster={mockCluster}
              isBreakdownLoading={true}
              isBreakdownSuccess={false}
            />
          </tbody>
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,test-runs-failed,failures',
      '/',
    );

    await screen.findByText(mockCluster.title);

    // Check the metric total values are displayed.
    for (let i = 0; i < metrics.length; i++) {
      const metricValues = mockCluster?.metrics || {};
      const metricValue = metricValues[metrics[i].metricId] || { value: '' };
      expect(screen.getByText(metricValue.value || '0')).toBeInTheDocument();
    }

    // Check there is a placeholder sparkline for each metric, and no actual sparklines.
    expect(
      screen.queryAllByTestId('clusters_table_sparkline_skeleton'),
    ).toHaveLength(metrics.length);
    expect(screen.queryAllByTestId('clusters_table_sparkline')).toHaveLength(0);
  });

  it('handles missing breakdown data', async () => {
    const metrics = getMockMetricsList('testproject');
    const mockCluster = getMockRuleBasicClusterSummary(
      'abcdef1234567890abcdef1234567890',
    );
    renderWithRouterAndClient(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <tbody>
            <ClustersTableRow
              project="testproject"
              cluster={mockCluster}
              isBreakdownLoading={false}
              isBreakdownSuccess={true}
            />
          </tbody>
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,test-runs-failed,failures',
      '/',
    );

    await screen.findByText(mockCluster.title);

    // Check the metric total values are displayed.
    for (let i = 0; i < metrics.length; i++) {
      const metricValues = mockCluster?.metrics || {};
      const metricValue = metricValues[metrics[i].metricId] || { value: '' };
      expect(screen.getByText(metricValue.value || '0')).toBeInTheDocument();
    }

    // Check there are neither placeholder sparklines, or actual sparklines.
    expect(
      screen.queryAllByTestId('clusters_table_sparkline_skeleton'),
    ).toHaveLength(0);
    expect(screen.queryAllByTestId('clusters_table_sparkline')).toHaveLength(0);
  });

  it('shows metric breakdowns', async () => {
    const metrics = getMockMetricsList('testproject');
    const mockCluster = getMockRuleFullClusterSummary(
      'abcdef1234567890abcdef1234567890',
    );
    renderWithRouterAndClient(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <tbody>
            <ClustersTableRow
              project="testproject"
              cluster={mockCluster}
              isBreakdownLoading={false}
              isBreakdownSuccess={true}
            />
          </tbody>
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,test-runs-failed,failures',
      '/',
    );

    await screen.findByText(mockCluster.title);

    // Check the metric total values are displayed.
    for (let i = 0; i < metrics.length; i++) {
      const metricValues = mockCluster?.metrics || {};
      const metricValue = metricValues[metrics[i].metricId] || { value: '' };
      expect(screen.getByText(metricValue.value || '0')).toBeInTheDocument();
    }

    // Check there are no placeholder sparklines.
    expect(
      screen.queryAllByTestId('clusters_table_sparkline_skeleton'),
    ).toHaveLength(0);

    // Check there are exactly 3 sparklines, for each metric.
    expect(screen.queryAllByTestId('clusters_table_sparkline')).toHaveLength(3);
  });
});
