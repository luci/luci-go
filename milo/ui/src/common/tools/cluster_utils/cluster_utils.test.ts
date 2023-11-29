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

import { Cluster } from '@/common/services/luci_analysis';

import { getClustersUniqueBugs } from './cluster_utils';

describe('cluster_utils', () => {
  const cluster1: Cluster = {
    clusterId: {
      algorithm: 'rule',
      id: 'cluster1',
    },
    bug: {
      system: 'monorail',
      id: '1234',
      linkText: 'crbug.com/1234',
      url: 'http://crbug.com/1234',
    },
  };

  const cluster2: Cluster = {
    clusterId: {
      algorithm: 'rule',
      id: 'cluster2',
    },
    bug: {
      system: 'monorail',
      id: '5678',
      linkText: 'crbug.com/5678',
      url: 'http://crbug.com/5678',
    },
  };

  const cluster3: Cluster = {
    clusterId: {
      algorithm: 'rule',
      id: 'cluster2',
    },
    bug: {
      system: 'buganizer',
      id: '1234',
      linkText: 'b/1234',
      url: 'http://b/1234',
    },
  };

  const cluster4: Cluster = {
    clusterId: {
      algorithm: 'rule',
      id: 'cluster2',
    },
    bug: {
      system: 'buganizer',
      id: '1234',
      linkText: 'b/1234',
      url: 'http://b/1234',
    },
  };

  it('getUniqueBugs should remove duplicate bugs', () => {
    const uniqueBugs = getClustersUniqueBugs([cluster1, cluster2, cluster3, cluster4]);
    expect(uniqueBugs.length).toBe(3);
  });
});
