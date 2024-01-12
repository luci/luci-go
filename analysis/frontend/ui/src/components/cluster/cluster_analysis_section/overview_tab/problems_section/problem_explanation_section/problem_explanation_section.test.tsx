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

import fetchMock from 'fetch-mock-jest';
import '@testing-library/jest-dom';
import {
  screen,
} from '@testing-library/react';

import { getMockMetricsList } from '@/testing_tools/mocks/metrics_mock';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { ClusterContextProvider } from '@/components/cluster/cluster_context';
import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { getMockCluster, mockGetCluster } from '@/testing_tools/mocks/cluster_mock';
import { createMockExonerationsPolicy } from '@/testing_tools/mocks/projects_mock';

import { Problem } from '@/tools/problems';
import { OverviewTabContextProvider } from '../../overview_tab_context';
import { ProblemExplanationSection } from './problem_explanation_section';

describe('Test ProblemExplanationSection component', () => {
  const project = 'testproject';
  const algorithm = 'rules';
  const id = '123456';

  beforeEach(() => {
    mockFetchAuthState();
    mockGetCluster(project, algorithm, id, getMockCluster(id, project, algorithm));
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  const metrics = getMockMetricsList('testproject');

  it('given an active problem, shows the deactivation criteria', async () => {
    const policy = createMockExonerationsPolicy();
    policy.metrics = [{
      metricId: 'critical-failures-exonerated',
      activationThreshold: {
        threeDay: '75',
        sevenDay: '100',
      },
      deactivationThreshold: {
        threeDay: '1',
        sevenDay: '2',
      },
    }];
    const problem : Problem = {
      policy: policy,
      state: {
        policyId: 'exonerations',
        isActive: true,
        lastActivationTime: '2022-02-01T02:34:56.123456Z',
        lastDeactivationTime: undefined,
      },
    };

    renderTabWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTabContextProvider metrics={metrics} >
            <ProblemExplanationSection problem={problem} />
          </OverviewTabContextProvider>
        </ClusterContextProvider>,
    );

    await screen.findByText('Problem description');

    // Status/priority chips.
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('P2')).toBeInTheDocument();

    // Problem description.
    expect(screen.getByText('Test variant(s) in this cluster are too flaky to gate changes in CQ. As a result, functionality is not protected from breakage.')).toBeInTheDocument();

    // Recommended actions.
    expect(screen.getByText('Review the exonerations tab to see which test variants are being exonerated.')).toBeInTheDocument();

    // Resolution criteria.
    expect(screen.getByText('Resolution Criteria')).toBeInTheDocument();
    await screen.findAllByText('Presubmit-blocking Failures Exonerated');
    expect(screen.getByText(/value:[ \n]+13800/)).toBeInTheDocument(); // Number of exonerated test failures in 7 days.
    expect(screen.getByText('AND')).toBeInTheDocument();
    expect(screen.getByText(/<[ \n]+1$/)).toBeInTheDocument(); // three day criteria.
    expect(screen.getByText(/<[ \n]+2$/)).toBeInTheDocument(); // seven day criteria.

    // Policy owners.
    expect(screen.getByText('exoneration-owner1@google.com, exoneration-owner2@google.com')).toBeInTheDocument();
  });

  it('given a resolved problem, shows the activation criteria', async () => {
    const policy = createMockExonerationsPolicy();
    policy.metrics = [{
      metricId: 'critical-failures-exonerated',
      activationThreshold: {
        threeDay: '75',
        sevenDay: '100',
      },
      deactivationThreshold: {
        threeDay: '1',
        sevenDay: '2',
      },
    }];
    const problem : Problem = {
      policy: policy,
      state: {
        policyId: 'exonerations',
        isActive: false,
        lastActivationTime: '2022-02-01T02:34:56.123456Z',
        lastDeactivationTime: '2022-02-01T11:34:56.123456Z',
      },
    };

    renderTabWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTabContextProvider metrics={metrics} >
            <ProblemExplanationSection problem={problem} />
          </OverviewTabContextProvider>
        </ClusterContextProvider>,
    );

    await screen.findByText('Problem description');

    // Status chip.
    expect(screen.getByText('Resolved')).toBeInTheDocument();

    // Problem description.
    expect(screen.getByText('Test variant(s) in this cluster are too flaky to gate changes in CQ. As a result, functionality is not protected from breakage.')).toBeInTheDocument();

    // Recommended actions.
    expect(screen.getByText('Review the exonerations tab to see which test variants are being exonerated.')).toBeInTheDocument();

    // Re-activation criteria.
    expect(screen.getByText('Re-activation Criteria')).toBeInTheDocument();
    await screen.findAllByText('Presubmit-blocking Failures Exonerated');
    expect(screen.getByText(/value:[ \n]+13800/)).toBeInTheDocument(); // Number of exonerated test failures in 7 days.
    expect(screen.getByText('OR')).toBeInTheDocument();
    expect(screen.getByText(/\u2265[ \n]+75/)).toBeInTheDocument(); // three day criteria.
    expect(screen.getByText(/\u2265[ \n]+100/)).toBeInTheDocument(); // seven day criteria.

    // Policy owners.
    expect(screen.getByText('exoneration-owner1@google.com, exoneration-owner2@google.com')).toBeInTheDocument();
  });
});
