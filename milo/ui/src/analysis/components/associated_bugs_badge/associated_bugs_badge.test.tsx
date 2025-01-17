// Copyright 2024 The LUCI Authors.
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

import { cleanup, render, screen } from '@testing-library/react';

import { OutputClusterEntry } from '@/analysis/types';

import { AssociatedBugsBadge } from './associated_bugs_badge';

const cluster1: OutputClusterEntry = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster1',
  },
  bug: {
    system: 'buganizer',
    id: '1234',
    linkText: 'b/1234',
    url: 'http://b/1234',
  },
};

const cluster2: OutputClusterEntry = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster2',
  },
  bug: {
    system: 'buganizer',
    id: '5678',
    linkText: 'b/5678',
    url: 'http://b/5678',
  },
};

const cluster3: OutputClusterEntry = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster3',
  },
  bug: {
    system: 'buganizer',
    id: '1234',
    linkText: 'b/1234',
    url: 'http://b/1234',
  },
};

describe('<AssociatedBugsBadge />', () => {
  afterEach(() => {
    cleanup();
  });

  it('should remove duplicated bugs', async () => {
    render(
      <AssociatedBugsBadge
        project="proj"
        clusters={[cluster1, cluster2, cluster3]}
      />,
    );

    const badges = screen.getAllByTestId('associated-bugs-badge');
    expect(badges).toHaveLength(2);
    expect(badges[0]).toHaveTextContent('b/1234');
    expect(badges[1]).toHaveTextContent('b/5678');
  });
});
