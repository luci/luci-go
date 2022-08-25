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

import { renderWithRouterAndClient } from '../../testing_tools/libs/mock_router';
import {
  createDefaultMockFailures,
  newMockFailure,
} from '../../testing_tools/mocks/failures_mock';
import { mockFetchAuthState } from '../../testing_tools/mocks/authstate_mock';
import { FailureFilters } from '../../tools/failures_tools';
import { mockQueryClusterFailures } from '../../testing_tools/mocks/cluster_mock';
import FailuresTable from './failures_table';

describe('Test FailureTable component', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given cluster failures, should group and display them', async () => {
    mockFetchAuthState();
    const mockFailures = createDefaultMockFailures();
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    expect(screen.getByText(mockFailures[0].testId!)).toBeInTheDocument();
  });

  it('when clicking a sortable column then should modify groups order', async () => {
    mockFetchAuthState();
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
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');

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
    mockFetchAuthState();
    const mockFailures = [
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
    ];
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');

    let allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(1);
    expect(allGroupCells[0]).toHaveTextContent('group1');

    await fireEvent.click(screen.getByLabelText('Expand group'));

    allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(4);
  });

  it('when filtering by failure type then should display matching groups', async () => {
    mockFetchAuthState();
    const mockFailures = [
      newMockFailure().withoutPresubmit().withTestId('group1').build(),
      newMockFailure().withTestId('group2').build(),
      newMockFailure().withTestId('group3').build(),
    ];
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');

    let allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(3);
    expect(allGroupCells[0]).toHaveTextContent('group1');
    expect(allGroupCells[1]).toHaveTextContent('group2');
    expect(allGroupCells[2]).toHaveTextContent('group3');

    await fireEvent.change(screen.getByTestId('failure_filter_input'), { target: { value: FailureFilters[1] } });

    allGroupCells = screen.getAllByTestId('failures_table_group_cell');
    expect(allGroupCells.length).toBe(2);
    expect(allGroupCells[0]).toHaveTextContent('group2');
    expect(allGroupCells[1]).toHaveTextContent('group3');
  });

  it('when filtering with impact then should recalculate impact', async () => {
    mockFetchAuthState();
    const mockFailures = [
      newMockFailure().withoutPresubmit().withTestId('group1').build(),
      newMockFailure().withTestId('group1').build(),
    ];
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');
    await fireEvent.change(screen.getByTestId('impact_filter_input'), { target: { value: 'Without Any Retries' } });

    let presubmitRejects = screen.getByTestId('failure_table_group_presubmitrejects');
    expect(presubmitRejects).toHaveTextContent('1');

    await fireEvent.change(screen.getByTestId('impact_filter_input'), { target: { value: 'Actual Impact' } });

    presubmitRejects = screen.getByTestId('failure_table_group_presubmitrejects');
    expect(presubmitRejects).toHaveTextContent('0');
  });

  it('when grouping by variants then should modify displayed tree', async () => {
    mockFetchAuthState();
    const mockFailures = [
      newMockFailure().withVariantGroups('v1', 'a').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'a').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'b').withTestId('group1').build(),
      newMockFailure().withVariantGroups('v1', 'b').withTestId('group1').build(),
    ];
    mockQueryClusterFailures('projects/chrome/clusters/rules/rule-123345/failures', mockFailures);

    renderWithRouterAndClient(
        <FailuresTable
          clusterAlgorithm="rules"
          clusterId="rule-123345"
          project="chrome"/>,
    );

    await screen.findByRole('table');
    await fireEvent.change(screen.getByTestId('group_by_input'), { target: { value: 'v1' } });

    const groupedCells = screen.getAllByTestId('failures_table_group_cell');
    expect(groupedCells.length).toBe(2);

    expect(groupedCells[0]).toHaveTextContent('a');
    expect(groupedCells[1]).toHaveTextContent('b');
  });
});
