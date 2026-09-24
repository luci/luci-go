// Copyright 2025 The LUCI Authors.
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

import { renderHook } from '@testing-library/react';

import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { Graph } from './build_tree';
import { useGraphSearch } from './use_graph_search';

const graph: Graph = {
  nodes: {
    root: {
      id: 'root',
      type: 'STAGE',
      label: 'root',
      status: 'FINAL',
      children: new Set(['child1', 'child2']),
      parents: new Set(),
      raw: Stage.fromPartial({
        identifier: { id: 'root', isWorknode: true },
        legacy: { worknode: { digest: 'digest-root-worknode' } },
      }),
    },
    child1: {
      id: 'child1',
      type: 'CHECK',
      label: 'child1',
      status: 'FINAL',
      children: new Set(['grandchild']),
      parents: new Set(['root']),
    },
    child2: {
      id: 'child2',
      type: 'CHECK',
      label: 'child2',
      status: 'UNKNOWN',
      children: new Set(),
      parents: new Set(['root']),
    },
    grandchild: {
      id: 'grandchild',
      type: 'CHECK',
      label: 'grandchild',
      status: 'FINAL',
      children: new Set(),
      parents: new Set(['child1']),
    },
  },
  roots: ['root'],
};

describe('useGraphSearch', () => {
  it('should return null when search query is empty', () => {
    const { result } = renderHook(() => useGraphSearch(graph, ''));
    expect(result.current.directMatches).toBeNull();
    expect(result.current.filteredNodeSet).toBeNull();
  });

  it('should return direct matches and filtered node set', () => {
    const { result } = renderHook(() => useGraphSearch(graph, 'child1'));
    expect(result.current.directMatches).toEqual(new Set(['child1']));
    expect(result.current.filteredNodeSet).toEqual(
      new Set(['root', 'child1', 'grandchild']),
    );
  });

  it('should handle no matches', () => {
    const { result } = renderHook(() => useGraphSearch(graph, 'no-match'));
    expect(result.current.directMatches).toEqual(new Set());
    expect(result.current.filteredNodeSet).toEqual(new Set());
  });

  it('should match stage worknode parameters resolved via valueDataMap', () => {
    const valueDataMap = new Map<string, ValueData>([
      [
        'digest-root-worknode',
        ValueData.fromPartial({
          json: {
            value: JSON.stringify({
              workExecutorType: 'PENDING_CHANGE_BUILD',
              workParameters: {
                pendingChangeBuild: {
                  changes: [{ changeNumber: '36481920' }],
                },
              },
            }),
          },
        }),
      ],
    ]);

    const { result } = renderHook(() =>
      useGraphSearch(graph, '36481920', valueDataMap),
    );
    expect(result.current.directMatches).toEqual(new Set(['root']));
    expect(result.current.filteredNodeSet).toEqual(
      new Set(['root', 'child1', 'child2', 'grandchild']),
    );
  });
});
