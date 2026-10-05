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

import { renderHook } from '@testing-library/react';
import { Edge } from '@xyflow/react';
import { useState } from 'react';

import { ChronicleNode } from '../../utils/graph_builder';

import {
  CONNECTED_MATCHED_NODE_STYLE,
  CONNECTED_NODE_STYLE,
  GraphLayout,
  MATCHED_NODE_STYLE,
  SELECTED_EDGE_STYLE,
  SELECTED_MATCHED_NODE_STYLE,
  SELECTED_NODE_STYLE,
  useGraphHighlighting,
} from './use_graph_highlighting';

const BASE_NODES: ChronicleNode[] = [
  {
    id: 'stage-1',
    position: { x: 0, y: 0 },
    style: { backgroundColor: '#ffffff' },
    data: { label: 'Stage 1', fullLabel: 'Stage 1' },
  },
  {
    id: 'check-1',
    position: { x: 100, y: 0 },
    style: { backgroundColor: '#f0f0f0' },
    data: { label: 'Check 1', fullLabel: 'Check 1' },
  },
  {
    id: 'check-2',
    position: { x: 200, y: 0 },
    style: { backgroundColor: '#e0e0e0' },
    data: { label: 'Check 2', fullLabel: 'Check 2' },
  },
];

const BASE_EDGES: Edge[] = [
  {
    id: 'e-stage1-check1',
    source: 'stage-1',
    target: 'check-1',
    style: { stroke: '#999999', strokeWidth: 1 },
    zIndex: 0,
  },
  {
    id: 'e-check1-check2',
    source: 'check-1',
    target: 'check-2',
    style: { stroke: '#999999', strokeWidth: 1 },
    zIndex: 0,
  },
];

const INITIAL_LAYOUT: GraphLayout = {
  nodes: BASE_NODES,
  edges: BASE_EDGES,
};

interface TestHookProps {
  baseLayout?: GraphLayout;
  effectiveSettledNodeId?: string;
  matchedNodeIds?: string[];
}

function useTestGraphHighlighting({
  baseLayout = INITIAL_LAYOUT,
  effectiveSettledNodeId = undefined,
  matchedNodeIds = [],
}: TestHookProps = {}) {
  const [nodes, setNodes] = useState<ChronicleNode[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);

  const { baseNodesMap } = useGraphHighlighting({
    baseLayout,
    effectiveSettledNodeId,
    matchedNodeIds,
    setNodes,
    setEdges,
  });

  return { nodes, edges, baseNodesMap, setNodes };
}

describe('useGraphHighlighting', () => {
  it('initializes nodes, edges, and baseNodesMap without highlights when nothing is selected or matched', () => {
    const { result } = renderHook(() => useTestGraphHighlighting());

    expect(result.current.baseNodesMap.size).toBe(3);
    expect(result.current.baseNodesMap.get('stage-1')).toBe(BASE_NODES[0]);
    expect(result.current.nodes).toEqual(BASE_NODES);
    expect(result.current.edges).toEqual(BASE_EDGES);
  });

  it('highlights selected node, immediate neighbors, and connecting edges (resolving stage attempt IDs)', () => {
    const { result } = renderHook(() =>
      useTestGraphHighlighting({
        effectiveSettledNodeId: 'L123:Sstage-1:A1',
      }),
    );

    const [nodeS1, nodeC1, nodeC2] = result.current.nodes;
    // stage-1 (selected) receives SELECTED_NODE_STYLE and check-1 (neighbor) receives CONNECTED_NODE_STYLE.
    expect(nodeS1.style).toEqual({
      ...BASE_NODES[0].style,
      ...SELECTED_NODE_STYLE,
    });
    expect(nodeC1.style).toEqual({
      ...BASE_NODES[1].style,
      ...CONNECTED_NODE_STYLE,
    });
    // check-2 is not a direct neighbor of stage-1, so its style remains unchanged.
    expect(nodeC2.style).toEqual(BASE_NODES[2].style);

    const [edgeS1C1, edgeC1C2] = result.current.edges;
    expect(edgeS1C1.style).toEqual({
      ...BASE_EDGES[0].style,
      ...SELECTED_EDGE_STYLE,
    });
    expect(edgeS1C1.zIndex).toBe(10);
    expect(edgeC1C2.style).toEqual(BASE_EDGES[1].style);
    expect(edgeC1C2.zIndex).toBe(0);
  });

  it('composes outer selection/neighbor borders with inner search match outline and glow for combined states', () => {
    const { result } = renderHook(() =>
      useTestGraphHighlighting({
        effectiveSettledNodeId: 'stage-1',
        matchedNodeIds: ['stage-1', 'check-1', 'check-2'],
      }),
    );

    const [nodeS1, nodeC1, nodeC2] = result.current.nodes;
    // stage-1 is both selected and matched -> SELECTED_MATCHED_NODE_STYLE
    expect(nodeS1.style).toEqual({
      ...BASE_NODES[0].style,
      ...SELECTED_MATCHED_NODE_STYLE,
    });
    // check-1 is both an immediate neighbor of stage-1 and matched -> CONNECTED_MATCHED_NODE_STYLE
    expect(nodeC1.style).toEqual({
      ...BASE_NODES[1].style,
      ...CONNECTED_MATCHED_NODE_STYLE,
    });
    // check-2 is only a search match -> MATCHED_NODE_STYLE
    expect(nodeC2.style).toEqual({
      ...BASE_NODES[2].style,
      ...MATCHED_NODE_STYLE,
    });
  });

  it('incrementally updates only changed elements and restores base styles when deselected', () => {
    const { result, rerender } = renderHook(
      (props: TestHookProps) => useTestGraphHighlighting(props),
      {
        initialProps: {
          effectiveSettledNodeId: 'stage-1',
          matchedNodeIds: ['check-2'],
        } as TestHookProps,
      },
    );

    const initialNodeS1 = result.current.nodes[0];
    const initialNodeC1 = result.current.nodes[1];
    const initialNodeC2 = result.current.nodes[2];
    const initialEdgeS1C1 = result.current.edges[0];
    const initialEdgeC1C2 = result.current.edges[1];

    // Clear selection while keeping check-2 matched: stage-1 and check-1 should revert to base style,
    // while check-2's object reference is preserved because its highlight state ('matched') did not change.
    rerender({
      effectiveSettledNodeId: undefined,
      matchedNodeIds: ['check-2'],
    });

    expect(result.current.nodes[0]).not.toBe(initialNodeS1);
    expect(result.current.nodes[0].style).toEqual(BASE_NODES[0].style);
    expect(result.current.nodes[1]).not.toBe(initialNodeC1);
    expect(result.current.nodes[1].style).toEqual(BASE_NODES[1].style);
    expect(result.current.nodes[2]).toBe(initialNodeC2);

    // Edge stage1-check1 reverts to base style and zIndex 0; edge check1-check2 reference is preserved.
    expect(result.current.edges[0]).not.toBe(initialEdgeS1C1);
    expect(result.current.edges[0].style).toEqual(BASE_EDGES[0].style);
    expect(result.current.edges[0].zIndex).toBe(0);
    expect(result.current.edges[1]).toBe(initialEdgeC1C2);
  });
});
