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

import {
  render,
  screen,
} from '@testing-library/react';

import { getMockCluster } from '@/testing_tools/mocks/cluster_mock';
import { getMockMetricsList } from '@/testing_tools/mocks/metrics_mock';

import ImpactTable from './impact_table';

describe('Test ImpactTable component', () => {
  it('given a cluster, should display it', async () => {
    const metrics = getMockMetricsList();
    const cluster = getMockCluster('1234567890abcdef1234567890abcdef');
    render(<ImpactTable cluster={cluster} metrics={metrics} />);

    await screen.findByText('User Cls Failed Presubmit');
    // Check for 7d unexpected failures total.
    expect(screen.getByText('15800')).toBeInTheDocument();

    // Check for 7d critical failures exonerated.
    expect(screen.getByText('13800')).toBeInTheDocument();
  });
});
