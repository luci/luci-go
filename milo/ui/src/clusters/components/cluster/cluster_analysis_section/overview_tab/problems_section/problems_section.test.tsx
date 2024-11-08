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

import {
  fireEvent,
  getByTestId,
  getByText,
  queryByTestId,
  screen,
  waitFor,
} from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { ClusterContextProvider } from '@/clusters/components/cluster/cluster_context';
import { renderTabWithRouterAndClient } from '@/clusters/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockGetCluster,
} from '@/clusters/testing_tools/mocks/cluster_mock';
import {
  getMockMetricsList,
  mockFetchMetrics,
} from '@/clusters/testing_tools/mocks/metrics_mock';
import {
  createMockProjectConfig,
  mockFetchProjectConfig,
} from '@/clusters/testing_tools/mocks/projects_mock';
import {
  createDefaultMockRule,
  mockFetchRule,
} from '@/clusters/testing_tools/mocks/rule_mock';

import { OverviewTabContextProvider } from '../overview_tab_context';

import { ProblemsSection } from './problems_section';

describe('Test ProblemSection component', () => {
  const project = 'testproject';
  const algorithm = 'rules';
  const id = '123456';
  const mockMetrics = getMockMetricsList(project);

  beforeEach(() => {
    mockFetchAuthState();
    mockFetchMetrics(project, mockMetrics);
    mockGetCluster(
      project,
      algorithm,
      id,
      getMockCluster(id, project, algorithm),
    );
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  const metrics = getMockMetricsList('testproject');

  it('given a rule with active or previously active problems, it should show those problems', async () => {
    const mockRule = createDefaultMockRule();
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    mockRule.bugManagementState!.policyState = [
      {
        policyId: 'exonerations',
        isActive: true,
        lastActivationTime: '2022-02-01T02:34:56.123456Z',
        lastDeactivationTime: undefined,
      },
      {
        policyId: 'cls-rejected',
        isActive: false,
        lastActivationTime: '2022-02-01T01:34:56.123456Z',
        lastDeactivationTime: '2022-02-01T02:04:56.123456Z',
      },
    ];
    mockFetchRule(mockRule);
    mockFetchProjectConfig(createMockProjectConfig());

    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <OverviewTabContextProvider metrics={metrics}>
          <ProblemsSection />
        </OverviewTabContextProvider>
      </ClusterContextProvider>,
    );

    await screen.findByTestId('problem-summary');
    const clsRejectedRow = screen.getByTestId('policy-row-cls-rejected');
    expect(clsRejectedRow).toBeInTheDocument();
    expect(
      getByText(clsRejectedRow, 'many CL(s) are being rejected'),
    ).toBeInTheDocument();
    expect(queryByTestId(clsRejectedRow, 'problem_priority_chip')).toBeNull();
    expect(
      getByTestId(clsRejectedRow, 'problem_status_chip'),
    ).toHaveTextContent('Resolved');

    const exonerationsRow = screen.getByTestId('policy-row-exonerations');
    expect(exonerationsRow).toBeInTheDocument();
    expect(
      getByText(
        exonerationsRow,
        'test variant(s) are being exonerated (ignored) in presubmit',
      ),
    ).toBeInTheDocument();
    expect(
      getByTestId(exonerationsRow, 'problem_priority_chip'),
    ).toHaveTextContent('P2');
    expect(
      getByTestId(exonerationsRow, 'problem_status_chip'),
    ).toHaveTextContent('Active');

    // Open dialog for one problem.
    fireEvent.click(
      getByText(screen.getByTestId('policy-row-cls-rejected'), 'more info'),
    );
    expect(
      await screen.findByText('Problem: many CL(s) are being rejected'),
    ).toBeInTheDocument();
    expect(
      await screen.findByText(
        'Many changelists are unable to submit because of failures on this test.',
      ),
    ).toBeInTheDocument();

    // Close dialog.
    fireEvent.click(screen.getByText('Close'));
    await waitFor(() => {
      expect(
        screen.queryByText('Problem: many CL(s) are being rejected'),
      ).toBeNull();
    });

    // Open dialog for the other problem.
    fireEvent.click(
      getByText(screen.getByTestId('policy-row-exonerations'), 'more info'),
    );
    expect(
      await screen.findByText(
        'Problem: test variant(s) are being exonerated (ignored) in presubmit',
      ),
    ).toBeInTheDocument();
    expect(
      await screen.findByText(
        'Test variant(s) in this cluster are too flaky to gate changes in CQ. As a result, functionality is not protected from breakage.',
      ),
    ).toBeInTheDocument();

    // Close dialog.
    fireEvent.click(screen.getByText('Close'));
    await waitFor(() => {
      expect(
        screen.queryByText(
          'Problem: test variant(s) are being exonerated (ignored) in presubmit',
        ),
      ).toBeNull();
    });
  });

  it('given a rule with no active or previously active problems, it should show placeholder text', async () => {
    const mockRule = createDefaultMockRule();
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    mockRule.bugManagementState!.policyState = [
      {
        policyId: 'exonerations',
        isActive: false,
        lastActivationTime: undefined,
        lastDeactivationTime: undefined,
      },
      {
        policyId: 'cls-rejected',
        isActive: false,
        lastActivationTime: undefined,
        lastDeactivationTime: undefined,
      },
    ];
    mockFetchRule(mockRule);
    mockFetchProjectConfig(createMockProjectConfig());

    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <OverviewTabContextProvider metrics={metrics}>
          <ProblemsSection />
        </OverviewTabContextProvider>
      </ClusterContextProvider>,
    );

    await screen.findByTestId('problem-summary');
    expect(
      screen.getByText('No problems have been identified.'),
    ).toBeInTheDocument();
  });

  it('given no policies configured in the project, it should show placeholder text', async () => {
    const mockRule = createDefaultMockRule();
    mockFetchRule(mockRule);

    const mockConfig = createMockProjectConfig();
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    mockConfig.bugManagement!.policies = [];
    mockFetchProjectConfig(mockConfig);

    renderTabWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <OverviewTabContextProvider metrics={metrics}>
          <ProblemsSection />
        </OverviewTabContextProvider>
      </ClusterContextProvider>,
    );

    await screen.findByTestId('problem-summary');
    expect(
      screen.getByText('Configure bug management policies'),
    ).toBeInTheDocument();
  });
});
