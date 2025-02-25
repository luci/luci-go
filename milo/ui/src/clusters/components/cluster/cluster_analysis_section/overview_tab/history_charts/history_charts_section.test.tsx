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

import '@testing-library/jest-dom';

import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { ClusterContextProvider } from '@/clusters/components/cluster/cluster_context';
import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import { mockQueryHistory } from '@/clusters/testing_tools/mocks/cluster_mock';
import { getMockMetricsList } from '@/clusters/testing_tools/mocks/metrics_mock';
import { QueryClusterHistoryResponse } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import { OverviewTabContextProvider } from '../overview_tab_context';

import { HistoryChartsSection } from './history_charts_section';

describe('test HistoryChartsSection component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  const metrics = getMockMetricsList('testproject');

  it('given cluster and metrics context, should fetch cluster history for that cluster', async () => {
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

    renderWithRouterAndClient(
      <ClusterContextProvider
        project="chrome"
        clusterAlgorithm="rules"
        clusterId="123456"
      >
        <OverviewTabContextProvider metrics={metrics}>
          <HistoryChartsSection />
        </OverviewTabContextProvider>
      </ClusterContextProvider>,
    );

    await screen.findAllByTestId('history-charts-container');

    // Expect charts only for the default metrics.
    metrics
      .filter((metric) => metric.isDefault)
      .forEach((metric) => {
        expect(
          screen.getByTestId('chart-' + metric.metricId),
        ).toBeInTheDocument();
      });
    metrics
      .filter((metric) => !metric.isDefault)
      .forEach((metric) => {
        expect(screen.queryByTestId('chart-' + metric.metricId)).toBeNull();
      });
  });

  it('unselecting a metric should exclude it from the cluster history', async () => {
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

    renderWithRouterAndClient(
      <ClusterContextProvider
        project="chrome"
        clusterAlgorithm="rules"
        clusterId="123456"
      >
        <OverviewTabContextProvider metrics={metrics}>
          <HistoryChartsSection />
        </OverviewTabContextProvider>
      </ClusterContextProvider>,
    );

    await screen.findAllByTestId('history-charts-container');

    // Check the chart for User Cls Failed Presubmit is displayed by default.
    expect(
      screen.queryByTestId('chart-human-cls-failed-presubmit'),
    ).toBeInTheDocument();

    const updatedHistory: QueryClusterHistoryResponse = {
      days: [
        {
          date: '2023-02-16',
          metrics: {
            'critical-failures-exonerated': 20,
            'test-runs-failed': 100,
          },
        },
      ],
    };
    mockQueryHistory(updatedHistory);

    // Deselect the metric "User Cls Failed Presubmit".
    await fireEvent.mouseDown(
      screen.getByRole('combobox', {
        name: 'Metrics',
      }),
    );
    const listOfItems = within(screen.getByRole('listbox'));
    fireEvent.click(listOfItems.getByText('User Cls Failed Presubmit'));

    // Check there is no chart for User Cls Failed Presubmit.
    await waitFor(() => {
      expect(
        screen.queryByTestId('chart-human-cls-failed-presubmit'),
      ).toBeNull();
    });
  });
});
