// Copyright 2026 The LUCI Authors.
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

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { buildSearchIndexMap } from './search_index_worker';

describe('search_index_worker', () => {
  it('builds a lowercase search index map for all nodes including ValueData payloads', () => {
    const nodes = [
      {
        id: 'check-1',
        label: 'Compile Android',
        view: Check.fromPartial({
          identifier: { id: 'check-1' },
          options: [{ digest: 'digest-1' }],
        }),
      },
      {
        id: 'stage-1',
        label: 'Run Tests',
      },
    ];
    const valueDataMap = new Map<string, ValueData>([
      [
        'digest-1',
        ValueData.fromPartial({
          json: { value: JSON.stringify({ target: 'aosp_arm64' }) },
        }),
      ],
    ]);

    const indexMap = buildSearchIndexMap(nodes, valueDataMap);

    expect(indexMap.size).toBe(2);
    expect(indexMap.get('check-1')).toContain('compile android');
    expect(indexMap.get('check-1')).toContain('aosp_arm64');
    expect(indexMap.get('stage-1')).toContain('run tests');
  });
});
