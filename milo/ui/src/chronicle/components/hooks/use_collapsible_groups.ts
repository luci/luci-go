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

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';

import { GroupMode, getTopologyGroups } from '../../utils/graph_builder';
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
 * @param graph The TurboCI WorkPlan data.
 * @returns An object containing the current collapsed state, pre-calculated group metadata, and manipulator actions.
 */
export function useCollapsibleGroups(graph: WorkPlan | undefined) {
  // Mapping of group ID -> group mode (expand/collapsed)
  const [groupModes, setGroupModes] = useState<Map<number, GroupMode>>(
    new Map(),
  );

  const groupData = useMemo(() => {
    if (!graph) {
      return {
        groupIdToChecks: new Map<number, Check[]>(),
        nodeToGroupId: new Map<string, number>(),
        parentToGroupIds: new Map<string, number[]>(),
      };
    }
    return getTopologyGroups(graph);
  }, [graph]);

  // Track if we have applied the defaults for the current graph instance to avoid
  // resetting user state on re-renders.
  const initializedGraphRef = useRef<WorkPlan | undefined>(undefined);

  // Initialize defaults: Collapse all successful groups when a new graph loads
  useEffect(() => {
    if (graph && initializedGraphRef.current !== graph) {
      const numChecks = Object.keys(graph.checks).length;
      if (numChecks > NUM_CHECKS_COLLAPSE_ALL) {
        const allModes = new Map<number, GroupMode>();
        for (const groupId of groupData.groupIdToChecks.keys()) {
          allModes.set(groupId, GroupMode.COLLAPSE_ALL);
        }
        setGroupModes(allModes);
      } else {
        setGroupModes(new Map());
      }
      initializedGraphRef.current = graph;
    }
  }, [graph, groupData]);

  // Helper to update mode for a list of group IDs
  const updateModes = (groupIds: number[], mode: GroupMode) => {
    setGroupModes((prev) => {
      const next = new Map(prev);
      groupIds.forEach((id) => next.set(id, mode));
      return next;
    });
  };

  const collapse = (groupIds: number[]) => {
    updateModes(groupIds, GroupMode.COLLAPSE_ALL);
  };

  const expand = (groupIds: number[]) => {
    updateModes(groupIds, GroupMode.EXPANDED);
  };

  const collapseAllSuccessful = () => {
    updateModes(
      Array.from(groupData.groupIdToChecks.keys()),
      GroupMode.COLLAPSE_SUCCESS_ONLY,
    );
  };

  const collapseAll = () => {
    updateModes(
      Array.from(groupData.groupIdToChecks.keys()),
      GroupMode.COLLAPSE_ALL,
    );
  };

  const expandAll = () => {
    updateModes(
      Array.from(groupData.groupIdToChecks.keys()),
      GroupMode.EXPANDED,
    );
  };

  return {
    groupModes,
    groupData,
    actions: {
      updateModes,
      collapse,
      expand,
      collapseAllSuccessful,
      collapseAll,
      expandAll,
    },
  };
}
