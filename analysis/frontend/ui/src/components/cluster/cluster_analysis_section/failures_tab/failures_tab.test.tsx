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
} from '@testing-library/react';

import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { mockQueryClusterFailures } from '@/testing_tools/mocks/cluster_mock';
import {
  createDefaultMockFailures,
  newMockFailure,
} from '@/testing_tools/mocks/failures_mock';

import { QueryClusterFailuresRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { mockFetchMetrics } from '@/testing_tools/mocks/metrics_mock';
import { ClusterContextProvider } from '../../cluster_context';
import FailuresTab from './failures_tab';

describe('Test FailureTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
    mockFetchMetrics('chrome');
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given cluster failures, should group and display them', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = createDefaultMockFailures();
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    await waitFor( () => expect(screen.getByText(mockFailures[0].testId!)).toBeInTheDocument());
  });

  it('when clicking a sortable column then should modify groups order', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = [
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group2').build(),
      newMockFailure().withTestId('group3').build(),
      newMockFailure().withTestId('group3').build(),
      newMockFailure().withTestId('group3').build(),
      newMockFailure().withTestId('group3').build(),
    ];
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    await screen.findAllByTestId('failures_table_group_cell');

    let allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(3);
    expect(allGroupCells[0]).toHaveTextContent('group1');
    expect(allGroupCells[1]).toHaveTextContent('group2');
    expect(allGroupCells[2]).toHaveTextContent('group3');

    await fireEvent.click(screen.getByText('Total Failures'));

    allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(3);
    expect(allGroupCells[0]).toHaveTextContent('group3');
    expect(allGroupCells[1]).toHaveTextContent('group1');
    expect(allGroupCells[2]).toHaveTextContent('group2');
  });

  it('when expanding then should show child groups', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = [
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
    ];
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    await screen.findAllByTestId('failures_table_group_cell');

    let allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(1);
    expect(allGroupCells[0]).toHaveTextContent('group1');

    await fireEvent.click(screen.getByLabelText('Expand group'));

    allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(4);
  });

  it('when filtering by failure type then should display matching groups', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = [
      newMockFailure().withoutPresubmit().withTestId('group1').build(),
      newMockFailure().withTestId('group2').build(),
      newMockFailure().withTestId('group3').build(),
    ];
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    await screen.findAllByTestId('failures_table_group_cell');

    let allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(3);
    expect(allGroupCells[0]).toHaveTextContent('group1');
    expect(allGroupCells[1]).toHaveTextContent('group2');
    expect(allGroupCells[2]).toHaveTextContent('group3');

    const failuresWithFilterRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: 'projects/chrome/metrics/critical-failures-exonerated',
    };
    const mockFailuresWithFilter = [
      newMockFailure().withTestId('group4').build(),
      newMockFailure().withTestId('group5').build(),
    ];
    mockQueryClusterFailures(failuresWithFilterRequest, mockFailuresWithFilter);

    await fireEvent.change(screen.getByTestId('failure_filter_input'), { target: { value: 'critical-failures-exonerated' } });

    await screen.findByRole('table');
    await screen.findAllByTestId('failures_table_group_cell');

    allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(2);
    expect(allGroupCells[0]).toHaveTextContent('group4');
    expect(allGroupCells[1]).toHaveTextContent('group5');
  });

  it('when filtering with impact then should recalculate impact', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = [
      newMockFailure().withoutPresubmit().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
    ];
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    await screen.findByTestId('failure_table_group_presubmitrejects');

    await fireEvent.change(screen.getByTestId('impact_filter_input'), { target: { value: 'without-retries' } });


    let presubmitRejects = screen.getByTestId('failure_table_group_presubmitrejects');
    expect(presubmitRejects).toHaveTextContent('1');

    await fireEvent.change(screen.getByTestId('impact_filter_input'), { target: { value: '' } });

    presubmitRejects = screen.getByTestId('failure_table_group_presubmitrejects');
    expect(presubmitRejects).toHaveTextContent('0');
  });

  it('when grouping by variants then should modify displayed tree', async () => {
    const failuresRequest: QueryClusterFailuresRequest = {
      parent: 'projects/chrome/clusters/rules/rule-123345/failures',
      metricFilter: '',
    };
    const mockFailures = [
      newMockFailure().withVariantGroups('v1', 'a').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'a').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'b').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'b').withTestId('group1').build(),
    ];
    mockQueryClusterFailures(failuresRequest, mockFailures);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='rule-123345'>
          <FailuresTab value='test' />,
        </ClusterContextProvider>,
    );

    await screen.findByRole('table');
    await fireEvent.change(screen.getByTestId('group_by_input'), { target: { value: 'v1' } });

    const groupedCells = screen.getAllByTestId('failures_table_group_cell');
    expect(groupedCells.length).toBe(2);

    expect(groupedCells[0]).toHaveTextContent('a');
    expect(groupedCells[1]).toHaveTextContent('b');
  });
});
