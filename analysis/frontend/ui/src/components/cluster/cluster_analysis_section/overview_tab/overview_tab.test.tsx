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

import { renderTabWithRouterAndClient } from '@/testing_tools/libs/render_tab';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  getMockCluster,
  mockBatchGetCluster,
} from '@/testing_tools/mocks/cluster_mock';

import { ClusterContextProvider } from '../../cluster_context';
import ImpactTab from './overview_tab';

describe('test ImpactSection component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given an algorithm, should fetch cluster for that algorithm', async () => {
    const cluster = getMockCluster('123456');

    mockBatchGetCluster('chrome', 'rules', '123456', cluster);

    renderTabWithRouterAndClient(
        <ClusterContextProvider project='chrome' clusterAlgorithm='rules' clusterId='123456'>
          <ImpactTab value='test' />
        </ClusterContextProvider>,
    );

    await screen.findByText('Impact');

    expect(screen.getByTestId('impact-table')).toBeInTheDocument();
  });
});
