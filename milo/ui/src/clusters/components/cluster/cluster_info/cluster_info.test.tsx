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
import fetchMock from 'fetch-mock-jest';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockGetCluster,
} from '@/clusters/testing_tools/mocks/cluster_mock';

import { ClusterContextProvider } from '../cluster_context';

import ClusterInfo from './cluster_info';

describe('test ClusterInfo component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('Given reason based cluster then should display the data', async () => {
    const project = 'chromium';
    const algorithm = 'reason-v3';
    const id = '14ee3dde813f66adc0595e4a21aa1743';
    const mockCluster = getMockCluster(
      id,
      project,
      algorithm,
      'ninja://chrome/android',
    );

    mockGetCluster(project, algorithm, id, mockCluster);

    renderWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <ClusterInfo />
      </ClusterContextProvider>,
    );

    await screen.findByText('Failure reason cluster');
    await screen.findByTestId('cluster-definition');

    expect(screen.getByText(mockCluster.title!)).toBeInTheDocument();
  });

  it('Given test based cluster then should display the data', async () => {
    const project = 'chromium';
    const algorithm = 'testname-v3';
    const id = '14ee3dde813f66adc0595e4a21aa1743';
    const mockCluster = getMockCluster(
      id,
      project,
      algorithm,
      'ninja://chrome/android',
    );

    mockGetCluster(project, algorithm, id, mockCluster);

    renderWithRouterAndClient(
      <ClusterContextProvider
        project={project}
        clusterAlgorithm={algorithm}
        clusterId={id}
      >
        <ClusterInfo />
      </ClusterContextProvider>,
    );

    await screen.findByText('Test name cluster');
    await screen.findByTestId('cluster-definition');

    expect(screen.getByText(mockCluster.title!)).toBeInTheDocument();
  });
});
