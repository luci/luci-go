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

import { fireEvent, screen, waitFor } from '@testing-library/react';

import { OverviewTabContextProvider } from '@/clusters/components/cluster/cluster_analysis_section/overview_tab/overview_tab_context';
import { renderWithRouter } from '@/clusters/testing_tools/libs/mock_router';
import { getMockMetricsList } from '@/clusters/testing_tools/mocks/metrics_mock';

import { HISTORY_TIME_RANGE_OPTIONS } from './constants';
import { HistoryChartsForm } from './history_charts_form';

describe('Test HistoryChartsForm component', () => {
  const metrics = getMockMetricsList('testproject');

  it('annotations can be toggled', async () => {
    renderWithRouter(
      <OverviewTabContextProvider>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
    );

    await screen.findAllByText('Annotate values');

    // Check annotations are off by default.
    const toggle = screen.getByRole('checkbox');
    expect(toggle).toHaveProperty('checked', false);

    await fireEvent.click(toggle);
    await waitFor(() => {
      expect(toggle).toHaveProperty('checked', true);
    });
  });

  it('should display all time range options', async () => {
    renderWithRouter(
      <OverviewTabContextProvider metrics={[]}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
    );

    await screen.findAllByText('Time Range');

    // Check all time ranges are available options.
    await fireEvent.mouseDown(
      screen.getByRole('combobox', {
        name: 'Time Range',
      }),
    );
    HISTORY_TIME_RANGE_OPTIONS.forEach((timeRange) => {
      expect(screen.getByText(timeRange.label)).toBeInTheDocument();
    });
  });

  it('given a history time range in the URL, the time range selection should match', async () => {
    const lastOption =
      HISTORY_TIME_RANGE_OPTIONS[HISTORY_TIME_RANGE_OPTIONS.length - 1];
    renderWithRouter(
      <OverviewTabContextProvider metrics={[]}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
      `/?historyTimeRange=${lastOption.id}`,
    );

    await screen.findAllByText('Time Range');
    expect(
      screen.getByTestId('history-charts-form-time-range-selection'),
    ).toHaveValue(lastOption.id);
  });

  it('should have no time range when given an unrecognized time range ID', async () => {
    renderWithRouter(
      <OverviewTabContextProvider metrics={[]}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
      '/?historyTimeRange=9d',
    );

    await screen.findAllByText('Time Range');
    expect(
      screen.getByTestId('history-charts-form-time-range-selection'),
    ).toHaveValue('');
  });

  it('should display all metrics options', async () => {
    renderWithRouter(
      <OverviewTabContextProvider metrics={metrics}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
    );

    await screen.findAllByText('Metrics');

    // Check all metrics are available options.
    await fireEvent.mouseDown(
      screen.getByRole('combobox', {
        name: 'Metrics',
      }),
    );
    metrics.forEach((metric) => {
      expect(screen.getByText(metric.humanReadableName)).toBeInTheDocument();
    });
  });

  it('given selected metrics in the URL, the selected metrics should match', async () => {
    const selectedMetrics = [metrics[0].metricId, metrics[1].metricId];
    renderWithRouter(
      <OverviewTabContextProvider metrics={metrics}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
      `/?selectedMetrics=${selectedMetrics.join(',')}`,
    );

    await screen.findAllByText('Metrics');
    expect(screen.getByTestId('metrics-selector')).toHaveValue(
      selectedMetrics.join(','),
    );
  });

  it('should have only known metrics when given an unrecognized metric ID', async () => {
    const selectedMetrics = [metrics[0].metricId, metrics[1].metricId];
    renderWithRouter(
      <OverviewTabContextProvider metrics={metrics}>
        <HistoryChartsForm />
      </OverviewTabContextProvider>,
      `/?selectedMetrics=testMetric,${selectedMetrics.join(',')}`,
    );

    await screen.findAllByText('Metrics');
    expect(screen.getByTestId('metrics-selector')).toHaveValue(
      selectedMetrics.join(','),
    );
  });
});
