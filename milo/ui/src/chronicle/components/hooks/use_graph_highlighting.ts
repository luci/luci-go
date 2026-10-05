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

import { Edge } from '@xyflow/react';
import { Dispatch, SetStateAction, useEffect, useMemo, useRef } from 'react';

import { ChronicleNode } from '../../utils/graph_builder';
import { getBaseNodeId } from '../../utils/id';

// We must explicitly set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
export const SELECTED_NODE_STYLE = {
  borderTop: '1px solid #0056b3',
  borderRight: '1px solid #0056b3',
  borderBottom: '1px solid #0056b3',
  borderLeft: '1px solid #0056b3',
  boxShadow: '0 0 0 4px #0056b3',
  zIndex: 15,
};

export const CONNECTED_NODE_STYLE = {
  borderTop: '1px solid #007bff',
  borderRight: '1px solid #007bff',
  borderBottom: '1px solid #007bff',
  borderLeft: '1px solid #007bff',
  boxShadow: '0 0 0 1px #007bff',
  zIndex: 12,
};

export const MATCHED_NODE_STYLE = {
  outline: '3px solid #eab308',
  outlineOffset: '-4px',
  boxShadow: '0 0 8px 6px rgba(234, 179, 8, 0.65)',
  zIndex: 11,
};

export const SELECTED_MATCHED_NODE_STYLE = {
  ...SELECTED_NODE_STYLE,
  ...MATCHED_NODE_STYLE,
  boxShadow: `${SELECTED_NODE_STYLE.boxShadow}, ${MATCHED_NODE_STYLE.boxShadow}`,
  zIndex: 15,
};

export const CONNECTED_MATCHED_NODE_STYLE = {
  ...CONNECTED_NODE_STYLE,
  ...MATCHED_NODE_STYLE,
  boxShadow: `${CONNECTED_NODE_STYLE.boxShadow}, ${MATCHED_NODE_STYLE.boxShadow}`,
  zIndex: 14,
};

export const SELECTED_EDGE_STYLE = {
  stroke: '#007bff',
  strokeWidth: 3,
  pointerEvents: 'none' as const,
};

export type NodeHighlightState =
  | 'selected'
  | 'connected'
  | 'matched'
  | 'selected-matched'
  | 'connected-matched'
  | 'none';

function getHighlightStyleOverlay(state: NodeHighlightState) {
  switch (state) {
    case 'selected':
      return SELECTED_NODE_STYLE;
    case 'connected':
      return CONNECTED_NODE_STYLE;
    case 'matched':
      return MATCHED_NODE_STYLE;
    case 'selected-matched':
      return SELECTED_MATCHED_NODE_STYLE;
    case 'connected-matched':
      return CONNECTED_MATCHED_NODE_STYLE;
    case 'none':
      return undefined;
  }
}

export interface GraphLayout {
  nodes: ChronicleNode[];
  edges: Edge[];
}

export interface UseGraphHighlightingOptions {
  /**
   * Unhighlighted base nodes and edges produced by the graph layout worker.
   */
  baseLayout: GraphLayout;
  /**
   * Settled selected node ID (may include prefix or attempt suffix, e.g. stage attempt ID).
   */
  effectiveSettledNodeId: string | undefined;
  /**
   * Node IDs matching the current search query.
   */
  matchedNodeIds: string[];
  /**
   * ReactFlow nodes state setter.
   */
  setNodes: Dispatch<SetStateAction<ChronicleNode[]>>;
  /**
   * ReactFlow edges state setter.
   */
  setEdges: Dispatch<SetStateAction<Edge[]>>;
}

export interface UseGraphHighlightingResult {
  /**
   * O(1) lookup map from node ID to the unhighlighted base ChronicleNode from the layout worker.
   * Used to retrieve a node's original style or view data without scanning ReactFlow state.
   */
  baseNodesMap: ReadonlyMap<string, ChronicleNode>;
}

/**
 * Manages ReactFlow node and edge highlighting for selection (including immediate
 * neighbors and connecting edges) and search matches.
 *
 * Uses precomputed lookup/adjacency maps and incremental highlight-state diffing so
 * selection and search updates only mutate the specific nodes and edges whose highlight
 * state changed, preserving ReactFlow's measured dimensions (`node.measured`) and object
 * references for all other elements.
 */
export function useGraphHighlighting({
  baseLayout,
  effectiveSettledNodeId,
  matchedNodeIds,
  setNodes,
  setEdges,
}: UseGraphHighlightingOptions): UseGraphHighlightingResult {
  // O(1) lookup map from node ID to the unhighlighted base ChronicleNode from the layout worker.
  // Used to quickly retrieve a node's original style (when un-highlighting) and view data
  // (for InspectorPanel) without scanning or depending on the mutable ReactFlow `nodes` state.
  const baseNodesMap = useMemo(
    () => new Map(baseLayout.nodes.map((node) => [node.id, node])),
    [baseLayout.nodes],
  );

  // O(1) lookup map from edge ID to the unhighlighted base Edge from the layout worker.
  // Used to restore an edge's original style and zIndex when it is no longer highlighted.
  const baseEdgesMap = useMemo(
    () => new Map(baseLayout.edges.map((edge) => [edge.id, edge])),
    [baseLayout.edges],
  );

  // Precomputed adjacency list mapping each node ID to its immediate neighbor node IDs
  // and connecting edge IDs. Avoids scanning all edges on every selection change.
  const adjacencyMap = useMemo(() => {
    const map = new Map<string, { neighborIds: string[]; edgeIds: string[] }>();
    for (const edge of baseLayout.edges) {
      let sourceEntry = map.get(edge.source);
      if (!sourceEntry) {
        sourceEntry = { neighborIds: [], edgeIds: [] };
        map.set(edge.source, sourceEntry);
      }
      sourceEntry.neighborIds.push(edge.target);
      sourceEntry.edgeIds.push(edge.id);

      let targetEntry = map.get(edge.target);
      if (!targetEntry) {
        targetEntry = { neighborIds: [], edgeIds: [] };
        map.set(edge.target, targetEntry);
      }
      targetEntry.neighborIds.push(edge.source);
      targetEntry.edgeIds.push(edge.id);
    }
    return map;
  }, [baseLayout.edges]);

  // Track the last baseLayout applied to nodes/edges and each element's previous highlight
  // state so setNodes/setEdges can update only the specific nodes/edges whose highlight state
  // changed, preserving ReactFlow's measured node dimensions (`node.measured`) and object references.
  const lastAppliedBaseLayoutRef = useRef<GraphLayout | undefined>(undefined);
  const lastNodeHighlightStateMapRef = useRef(
    new Map<string, NodeHighlightState>(),
  );
  const lastEdgeHighlightSetRef = useRef(new Set<string>());

  useEffect(() => {
    const matchedNodeSet = new Set(matchedNodeIds);
    const connectedNodeIds = new Set<string>();
    const connectedEdgeIds = new Set<string>();
    let selectedBaseNodeId: string | undefined;

    if (effectiveSettledNodeId) {
      selectedBaseNodeId = getBaseNodeId(effectiveSettledNodeId, {
        includePrefix: false,
      });
      if (selectedBaseNodeId) {
        // Find immediate neighbors and connecting edges using precomputed adjacency map
        const adj = adjacencyMap.get(selectedBaseNodeId);
        if (adj) {
          for (const neighborId of adj.neighborIds) {
            if (neighborId !== selectedBaseNodeId) {
              connectedNodeIds.add(neighborId);
            }
          }
          for (const edgeId of adj.edgeIds) {
            connectedEdgeIds.add(edgeId);
          }
        }
      }
    }

    const isNewLayout = lastAppliedBaseLayoutRef.current !== baseLayout;
    const prevNodeHighlightMap = lastNodeHighlightStateMapRef.current;
    const prevEdgeHighlightSet = lastEdgeHighlightSetRef.current;

    const nextNodeHighlightMap = new Map<string, NodeHighlightState>();
    for (const node of baseLayout.nodes) {
      const isSelected = node.id === selectedBaseNodeId;
      const isConnected = connectedNodeIds.has(node.id);
      const isMatched = matchedNodeSet.has(node.id);

      let targetState: NodeHighlightState = 'none';
      if (isSelected && isMatched) {
        targetState = 'selected-matched';
      } else if (isSelected) {
        targetState = 'selected';
      } else if (isConnected && isMatched) {
        targetState = 'connected-matched';
      } else if (isConnected) {
        targetState = 'connected';
      } else if (isMatched) {
        targetState = 'matched';
      }
      nextNodeHighlightMap.set(node.id, targetState);
    }

    setNodes((prevNodes) => {
      if (isNewLayout || prevNodes.length !== baseLayout.nodes.length) {
        return baseLayout.nodes.map((node) => {
          const targetState = nextNodeHighlightMap.get(node.id) ?? 'none';
          const overlay = getHighlightStyleOverlay(targetState);
          if (overlay) {
            return {
              ...node,
              style: { ...node.style, ...overlay },
            };
          }
          return node;
        });
      }

      let changed = false;
      const nextNodes = prevNodes.map((node) => {
        const targetState = nextNodeHighlightMap.get(node.id) ?? 'none';
        const prevState = prevNodeHighlightMap.get(node.id) ?? 'none';

        if (prevState === targetState) {
          return node;
        }

        changed = true;
        const baseNode = baseNodesMap.get(node.id);
        const baseStyle = baseNode?.style;
        const overlay = getHighlightStyleOverlay(targetState);

        if (overlay) {
          return {
            ...node,
            style: { ...baseStyle, ...overlay },
          };
        }
        return {
          ...node,
          style: baseStyle,
        };
      });

      return changed ? nextNodes : prevNodes;
    });

    setEdges((prevEdges) => {
      if (isNewLayout || prevEdges.length !== baseLayout.edges.length) {
        if (connectedEdgeIds.size === 0) {
          return baseLayout.edges;
        }
        return baseLayout.edges.map((edge) => {
          if (connectedEdgeIds.has(edge.id)) {
            return {
              ...edge,
              style: { ...edge.style, ...SELECTED_EDGE_STYLE },
              zIndex: 10,
            };
          }
          return edge;
        });
      }

      let changed = false;
      const nextEdges = prevEdges.map((edge) => {
        const wasHighlighted = prevEdgeHighlightSet.has(edge.id);
        const isHighlighted = connectedEdgeIds.has(edge.id);
        if (wasHighlighted === isHighlighted) {
          return edge;
        }
        changed = true;
        const baseEdge = baseEdgesMap.get(edge.id);
        if (isHighlighted) {
          return {
            ...edge,
            style: { ...baseEdge?.style, ...SELECTED_EDGE_STYLE },
            zIndex: 10,
          };
        }
        return {
          ...edge,
          style: baseEdge?.style,
          zIndex: baseEdge?.zIndex ?? 0,
        };
      });

      return changed ? nextEdges : prevEdges;
    });

    lastAppliedBaseLayoutRef.current = baseLayout;
    lastNodeHighlightStateMapRef.current = nextNodeHighlightMap;
    lastEdgeHighlightSetRef.current = connectedEdgeIds;
  }, [
    baseLayout,
    baseNodesMap,
    baseEdgesMap,
    adjacencyMap,
    effectiveSettledNodeId,
    matchedNodeIds,
    setNodes,
    setEdges,
  ]);

  return { baseNodesMap };
}
