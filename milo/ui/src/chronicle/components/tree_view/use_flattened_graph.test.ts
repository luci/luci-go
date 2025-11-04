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

import { Graph } from './build_tree';
import { useFlattenedGraph } from './use_flattened_graph';

const graph: Graph = {
  nodes: {
    root: {
      id: 'root',
      type: 'STAGE',
      label: 'root',
      status: 'FINAL',
      children: new Set(['child1', 'child2']),
      parents: new Set(),
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
      status: 'ATTEMPTING',
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

describe('useFlattenedGraph', () => {
  it('should return flattened graph', () => {
    const expandedIds = new Set(['root', 'child1']);
    const { result } = renderHook(() =>
      useFlattenedGraph(graph, expandedIds, null, null),
    );
    expect(result.current.map((i) => i.id)).toEqual([
      'root',
      'child1',
      'grandchild',
      'child2',
    ]);
  });

  it('should filter nodes', () => {
    const expandedIds = new Set(['root', 'child1']);
    const filteredNodeSet = new Set(['root', 'child1', 'grandchild']);
    const { result } = renderHook(() =>
      useFlattenedGraph(graph, expandedIds, filteredNodeSet, null),
    );
    expect(result.current.map((i) => i.id)).toEqual([
      'root',
      'child1',
      'grandchild',
    ]);
  });
});
