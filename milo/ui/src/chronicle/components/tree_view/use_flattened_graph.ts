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

import { useMemo } from 'react';

import { FlatTreeItem, Graph } from './build_tree';

/**
 * Traverses the graph and attaches display-relevant metadata to nodes
 *
 * @param graph The graph
 * @param expandedIds The nodes which are expanded by the user
 * @param filteredNodeSet All nodes which in the current filtered view
 * @param directMatches The nodes which directly match the filter query
 * @returns Flattened rows of the Tree view
 */
export function useFlattenedGraph(
  graph: Graph,
  expandedIds: Set<string>,
  filteredNodeSet: Set<string> | null,
  directMatches: Set<string> | null,
) {
  return useMemo(() => {
    const items: FlatTreeItem[] = [];
    const globallyVisited = new Set<string>();

    const traverse = (
      nodeId: string,
      depth: number,
      path: Set<string>,
      parentId: string | null,
    ) => {
      // Filter check: skip if a filter is active and node is not in allowed set
      if (filteredNodeSet && !filteredNodeSet.has(nodeId)) return;

      const node = graph.nodes[nodeId];
      if (!node) return;

      const isCycle = path.has(nodeId);
      const isRepeated = globallyVisited.has(nodeId);
      globallyVisited.add(nodeId);

      const children = Array.from(node.children || []).sort();
      const hasChildren = children.length > 0;
      const isMatch = directMatches ? directMatches.has(nodeId) : false;

      const key = `${parentId || 'root'}-${nodeId}-${items.length}`;

      items.push({
        id: nodeId,
        key,
        depth,
        isRepeated,
        isCycle,
        hasChildren,
        parentId,
        isMatch,
      });

      if (
        !isCycle &&
        (!isRepeated || expandedIds.has(nodeId)) &&
        expandedIds.has(nodeId)
      ) {
        const newPath = new Set(path).add(nodeId);
        for (const childId of children) {
          traverse(childId, depth + 1, newPath, nodeId);
        }
      }
    };

    graph.roots.forEach((rootId) => traverse(rootId, 0, new Set(), null));
    return items;
  }, [graph, expandedIds, filteredNodeSet, directMatches]);
}
