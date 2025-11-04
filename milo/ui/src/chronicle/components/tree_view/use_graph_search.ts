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

import { useEffect, useMemo, useRef } from 'react';

import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { Graph, transitiveDescendants } from './build_tree';

function isStageView(view: CheckView | StageView): view is StageView {
  return (view as StageView).stage !== undefined;
}

/**
 * Hook for providing matched and filtered nodes for a typed query.
 * @param graph The graph
 * @param searchQuery The typed filter query
 * @returns Nodes which match the query, and all "filtered" nodes (which are
 *   upstream/downstream of matches)
 */
export function useGraphSearch(graph: Graph, searchQuery: string) {
  // Lazy cache for full-text search strings of nodes.
  // Only populated when a search is first initiated.
  const searchIndexRef = useRef<Record<string, string> | null>(null);

  // Reset search index if the graph data itself changes entirely
  useEffect(() => {
    searchIndexRef.current = null;
  }, [graph]);

  // 1. Find all nodes that directly match the query (searching raw JSON data)
  const directMatches = useMemo(() => {
    if (!searchQuery.trim()) return null;
    const query = searchQuery.toLowerCase();

    // Lazily build the index on first search
    if (!searchIndexRef.current) {
      searchIndexRef.current = {};
      const nodes = Object.values(graph.nodes);
      for (let i = 0; i < nodes.length; i++) {
        const node = nodes[i];
        // Create a searchable string containing label and full raw JSON data.
        // For stage nodes, we exclude the `assignments` field. This is
        // because assignment IDs are not useful for search and can lead to
        // a single search query matching many nodes.
        searchIndexRef.current[node.id] = (
          node.label +
          ' ' +
          (node.raw
            ? node.type === 'STAGE' && isStageView(node.raw)
              ? JSON.stringify({
                  ...node.raw,
                  stage: { ...node.raw.stage, assignments: undefined },
                })
              : JSON.stringify(node.raw)
            : '')
        ).toLowerCase();
      }
    }

    const index = searchIndexRef.current;
    const matches = new Set<string>();
    for (const id in index) {
      if (index[id].includes(query)) {
        matches.add(id);
      }
    }
    return matches;
  }, [graph, searchQuery]);

  // 2. Expand matches to include necessary context (ancestors) and results (subtree)
  const filteredNodeSet = useMemo(() => {
    if (!directMatches) return null;
    if (directMatches.size === 0) return new Set<string>();

    // Find ancestors (to show context up to roots)
    const ancestors = new Set<string>();
    const stack = Array.from(directMatches);
    while (stack.length) {
      const id = stack.pop()!;
      graph.nodes[id]?.parents.forEach((p) => {
        if (!ancestors.has(p)) {
          ancestors.add(p);
          stack.push(p);
        }
      });
    }

    // Find descendants (to show subtrees of matches)
    // cast to string[] specifically because directMatches is a Set<string>
    const descendants = transitiveDescendants(
      graph,
      Array.from(directMatches) as string[],
    );

    return new Set([...directMatches, ...ancestors, ...descendants]);
  }, [graph, directMatches]);

  return { directMatches, filteredNodeSet };
}
