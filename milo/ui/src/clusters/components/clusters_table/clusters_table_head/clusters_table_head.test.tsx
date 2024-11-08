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

import { screen, waitFor } from '@testing-library/react';

import { ClusterTableContextWrapper } from '@/clusters/components/clusters_table/clusters_table_context';
import { renderWithRouter } from '@/clusters/testing_tools/libs/mock_router';
import { getMockMetricsList } from '@/clusters/testing_tools/mocks/metrics_mock';

import ClustersTableHead from './clusters_table_head';

describe('Test ClustersTableHead', () => {
  it('should display sortable table head', async () => {
    const metrics = getMockMetricsList('testproject');
    renderWithRouter(
      <ClusterTableContextWrapper metrics={metrics}>
        <table>
          <ClustersTableHead />
        </table>
      </ClusterTableContextWrapper>,
      '/?selectedMetrics=human-cls-failed-presubmit,critical-failures-exonerated,failures',
    );

    await screen.findByTestId('clusters_table_head');

    await waitFor(() => {
      expect(screen.getByText('User Cls Failed Presubmit')).toBeInTheDocument();
      expect(
        screen.getByText('Presubmit-blocking Failures Exonerated'),
      ).toBeInTheDocument();
      expect(screen.getByText('Total Failures')).toBeInTheDocument();
    });
  });
});
