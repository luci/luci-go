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

import { useCallback, useEffect, useRef, useState } from 'react';

import { Graph } from './build_tree';
import { useFlattenedGraph } from './use_flattened_graph';
import { useGraphSearch } from './use_graph_search';
import { useKeyboardNavigation } from './use_keyboard_navigation';

export interface UseTreeProps {
  graph: Graph;
  defaultExpandedDepth?: number;
}

/**
 Manages the state and interactions of a tree-like structure for displaying the
 graph. It handles expansion/collapse, keyboard navigation, and
 search/filtering.
 */
export function useTree({ graph, defaultExpandedDepth = 2 }: UseTreeProps) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const initializedRef = useRef(false);
  if (!initializedRef.current && graph.roots.length > 0) {
    initializedRef.current = true;
    const initialExpanded = new Set<string>();
    const stack = graph.roots.map((r) => ({ id: r, depth: 0 }));
    while (stack.length) {
      const { id, depth } = stack.pop()!;
      if (depth < defaultExpandedDepth) {
        initialExpanded.add(id);
        graph.nodes[id]?.children.forEach((c) =>
          stack.push({ id: c, depth: depth + 1 }),
        );
      }
    }
    setExpandedIds(initialExpanded);
  }

  const { directMatches, filteredNodeSet } = useGraphSearch(graph, searchQuery);

  // Auto-expand relevant nodes when searching
  useEffect(() => {
    if (directMatches && directMatches.size > 0) {
      // Expand matches and all their ancestors so they are visible
      const toExpand = new Set<string>(directMatches);
      const stack = Array.from(directMatches);
      while (stack.length) {
        const id = stack.pop()!;
        graph.nodes[id]?.parents.forEach((p) => {
          if (!toExpand.has(p)) {
            toExpand.add(p);
            stack.push(p);
          }
        });
      }
      setExpandedIds(toExpand);
    }
  }, [graph, directMatches]);

  const visibleItems = useFlattenedGraph(
    graph,
    expandedIds,
    filteredNodeSet,
    directMatches,
  );

  // Ensure initial selection matches something visible
  useEffect(() => {
    // If selection is lost (e.g. due to filter), select the first visible item
    if (
      visibleItems.length > 0 &&
      (!selectedKey || !visibleItems.some((i) => i.key === selectedKey))
    ) {
      setSelectedKey(visibleItems[0].key);
    }
  }, [visibleItems, selectedKey]);

  const { handleKeyDown } = useKeyboardNavigation(
    visibleItems,
    selectedKey,
    setSelectedKey,
    expandedIds,
    setExpandedIds,
    graph,
  );

  const toggleKey = useCallback(
    (key: string) => {
      const item = visibleItems.find((i) => i.key === key);
      if (item) {
        setExpandedIds((prev) => {
          const next = new Set(prev);
          if (next.has(item.id)) next.delete(item.id);
          else next.add(item.id);
          return next;
        });
      }
    },
    [visibleItems],
  );

  return {
    visibleItems,
    selectedKey,
    setSelectedKey,
    expandedIds,
    toggleKey,
    handleKeyDown,
    searchQuery,
    setSearchQuery,
  };
}
