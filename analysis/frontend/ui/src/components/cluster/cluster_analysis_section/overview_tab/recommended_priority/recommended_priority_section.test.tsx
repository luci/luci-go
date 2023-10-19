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

import fetchMock from 'fetch-mock-jest';

import { screen } from '@testing-library/react';

import { ProjectConfig } from '@/services/project';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockGetCluster,
} from '@/testing_tools/mocks/cluster_mock';
import { getMockMetricsList } from '@/testing_tools/mocks/metrics_mock';
import {
  createMockProjectConfig,
  createMockProjectConfigWithBuganizerThresholds,
  createMockProjectConfigWithMonorailThresholds,
  mockFetchProjectConfig,
} from '@/testing_tools/mocks/projects_mock';

import { ClusterContextProvider } from '../../../cluster_context';
import { OverviewTabContextProvider } from '../overview_tab_context';
import { RecommendedPrioritySection } from './recommended_priority_section';

describe('test RecommendedPrioritySection component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  const metrics = getMockMetricsList('chrome');
  const project = 'chrome';
  const algorithm = 'rules';
  const id = '123456';
  const mockCluster = getMockCluster(id, project, algorithm);

  it('shows the recommended priority', async () => {
    mockGetCluster(project, algorithm, id, mockCluster);

    const mockConfig = createMockProjectConfigWithBuganizerThresholds();
    mockFetchProjectConfig(mockConfig);

    renderWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTabContextProvider metrics={metrics} >
            <RecommendedPrioritySection />
          </OverviewTabContextProvider>
        </ClusterContextProvider>,
    );

    await screen.findAllByTestId('recommended-priority-summary');

    expect(screen.getByText('P0')).toBeInTheDocument();
    expect(screen.getByText('User Cls Failed Presubmit')).toBeInTheDocument();
    expect(screen.getByText('(1d) (value: 98) \u2265 20')).toBeInTheDocument();
    expect(screen.getByText('more info')).toBeInTheDocument();
  });

  it('adds the priority prefix for a numeric priority value', async () => {
    mockGetCluster(project, algorithm, id, mockCluster);

    const mockConfig = createMockProjectConfigWithMonorailThresholds();
    mockFetchProjectConfig(mockConfig);

    renderWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTabContextProvider metrics={metrics} >
            <RecommendedPrioritySection />
          </OverviewTabContextProvider>
        </ClusterContextProvider>,
    );

    await screen.findAllByTestId('recommended-priority-summary');

    expect(screen.getByText('P0')).toBeInTheDocument();
  });

  it('renders even without a recommended priority', async () => {
    mockGetCluster(project, algorithm, id, mockCluster);

    const mockConfig = createMockProjectConfig();
    mockFetchProjectConfig(mockConfig);

    renderWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <OverviewTabContextProvider metrics={metrics} >
            <RecommendedPrioritySection />
          </OverviewTabContextProvider>
        </ClusterContextProvider>,
    );

    await screen.findAllByTestId('recommended-priority-summary');

    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('more info')).toBeInTheDocument();
  });

  it('handles project config without monorail details', async () => {
    mockGetCluster(project, algorithm, id, mockCluster);

    const mockConfig: ProjectConfig = {
      name: 'projects/chromium/config',
      bugManagement: {},
    };
    mockFetchProjectConfig(mockConfig);

    renderWithRouterAndClient(
        <ClusterContextProvider
          project={project}
          clusterAlgorithm={algorithm}
          clusterId={id} >
          <RecommendedPrioritySection />
        </ClusterContextProvider>,
    );

    await screen.findAllByTestId('recommended-priority-summary');

    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('more info')).toBeInTheDocument();
  });
});
