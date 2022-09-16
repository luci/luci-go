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

import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import {
  getMockRuleClusterSummary,
  getMockSuggestedClusterSummary,
} from '@/testing_tools/mocks/cluster_mock';

import ClustersTableRow from './clusters_table_row';

describe('Test ClustersTableRow component', () => {
  it('given a rule cluster', async () => {
    const mockCluster = getMockRuleClusterSummary('abcdef1234567890abcdef1234567890');
    renderWithRouterAndClient(
        <table>
          <tbody>
            <ClustersTableRow
              project='testproject'
              cluster={mockCluster}/>
          </tbody>
        </table>,
    );

    await screen.findByText(mockCluster.title);

    expect(screen.getByText(mockCluster.bug?.linkText || '')).toBeInTheDocument();
    expect(screen.getByText(mockCluster.presubmitRejects || '0')).toBeInTheDocument();
    expect(screen.getByText(mockCluster.criticalFailuresExonerated || '0')).toBeInTheDocument();
    expect(screen.getByText(mockCluster.failures || '0')).toBeInTheDocument();
  });

  it('given a suggested cluster', async () => {
    const mockCluster = getMockSuggestedClusterSummary('abcdef1234567890abcdef1234567890');
    renderWithRouterAndClient(
        <table>
          <tbody>
            <ClustersTableRow
              project='testproject'
              cluster={mockCluster}/>
          </tbody>
        </table>,
    );

    await screen.findByText(mockCluster.title);

    expect(screen.getByText(mockCluster.presubmitRejects || '0')).toBeInTheDocument();
    expect(screen.getByText(mockCluster.criticalFailuresExonerated || '0')).toBeInTheDocument();
    expect(screen.getByText(mockCluster.failures || '0')).toBeInTheDocument();
  });
});
