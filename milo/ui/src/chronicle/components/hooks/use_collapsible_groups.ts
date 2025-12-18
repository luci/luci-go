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

import { useEffect, useMemo, useRef, useState } from 'react';

import { GraphView } from '@/proto/turboci/graph/orchestrator/v1/graph_view.pb';

import { CheckResultStatus } from '../../utils/check_utils';
import { getCollapsibleGroups } from '../../utils/graph_builder';

/**
 * The number of checks in the graph where we opt into collapsing everything by default on page load for performance reasons.
 */
const NUM_CHECKS_COLLAPSE_ALL = 500;

/**
 * Manages the logic and state for collapsing groups of nodes in the graph.
 *
 * This hook:
 * 1. Determines which groups are collapsible.
 * 2. Maintains the state of which groups are currently collapsed.
 * 3. Initializes the state by collapsing all successful groups when a new graph is loaded.
 * 4. Provides actions to expand/collapse specific groups or sets of groups.
 *
 * @param graph The TurboCI GraphView data.
 * @returns An object containing the current collapsed state, pre-calculated group metadata, and manipulator actions.
 */
export function useCollapsibleGroups(graph: GraphView | undefined) {
  const [collapsedHashes, setCollapsedHashes] = useState<Set<number>>(
    new Set(),
  );

  const groupData = useMemo(() => {
    if (!graph) {
      return {
        hashToGroup: new Map<number, string[]>(),
        hashToStatus: new Map<number, CheckResultStatus>(),
        parentToGroupHashes: new Map<string, number[]>(),
      };
    }
    return getCollapsibleGroups(graph);
  }, [graph]);

  // Track if we have applied the defaults for the current graph instance to avoid
  // resetting user state on re-renders.
  const initializedGraphRef = useRef<GraphView | undefined>(undefined);

  // Initialize defaults: Collapse all successful groups when a new graph loads
  useEffect(() => {
    if (graph && initializedGraphRef.current !== graph) {
      const numChecks = Object.keys(graph.checks).length;
      const initialCollapsed = new Set<number>();
      groupData.hashToGroup.forEach((_, hash) => {
        if (
          numChecks > NUM_CHECKS_COLLAPSE_ALL ||
          groupData.hashToStatus.get(hash) === CheckResultStatus.SUCCESS
        ) {
          initialCollapsed.add(hash);
        }
      });
      setCollapsedHashes(initialCollapsed);
      initializedGraphRef.current = graph;
    }
  }, [graph, groupData]);

  // Create state manipulator callbacks
  const collapse = (hashes: number[]) => {
    setCollapsedHashes((prev) => {
      const next = new Set(prev);
      hashes.forEach((h) => next.add(h));
      return next;
    });
  };

  const expand = (hashes: number[]) => {
    setCollapsedHashes((prev) => {
      const next = new Set(prev);
      hashes.forEach((h) => next.delete(h));
      return next;
    });
  };

  const collapseAllSuccessful = () => {
    const successHashes = new Set<number>();
    groupData.hashToGroup.forEach((_, hash) => {
      if (groupData.hashToStatus.get(hash) === CheckResultStatus.SUCCESS) {
        successHashes.add(hash);
      }
    });
    setCollapsedHashes(successHashes);
  };

  const collapseAll = () => {
    setCollapsedHashes(new Set(groupData.hashToGroup.keys()));
  };

  const expandAll = () => {
    setCollapsedHashes(new Set());
  };

  return {
    collapsedHashes,
    groupData,
    actions: {
      collapse,
      expand,
      collapseAllSuccessful,
      collapseAll,
      expandAll,
    },
  };
}
