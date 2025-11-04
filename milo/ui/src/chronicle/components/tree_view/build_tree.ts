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

import { checkStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { stageStateToJSON } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

// Note; this enum may be incomplete if stage state or check state gets updated with new values.
// (that's ok for now)
export type NodeStatus =
  | 'UNKNOWN'
  | 'PLANNED'
  | 'PLANNING'
  | 'WAITING'
  | 'ATTEMPTING'
  | 'AWAITING_GROUP'
  | 'FINAL';
export type NodeType = 'CHECK' | 'STAGE';

export interface GraphNode {
  id: string;
  type: NodeType;
  label: string;
  status: NodeStatus;
  children: Set<string>;
  parents: Set<string>;
  raw?: CheckView | StageView;
}

export interface Graph {
  nodes: Record<string, GraphNode>;
  roots: string[];
}

// Provides some less verbose versions of CHECK_STATE_x and STAGE_STATE_x
const checkStateToStatus = (state: number | undefined): NodeStatus => {
  if (state === undefined) return 'UNKNOWN';
  const stateStr = checkStateToJSON(state);
  return stateStr.replace('CHECK_STATE_', '') as NodeStatus;
};

const stageStateToStatus = (state: number | undefined): NodeStatus => {
  if (state === undefined) return 'UNKNOWN';
  const stateStr = stageStateToJSON(state);
  return stateStr.replace('STAGE_STATE_', '') as NodeStatus;
};

export function buildVisualGraph(
  stages: StageView[],
  checks: CheckView[],
): Graph {
  const nodes: Record<string, GraphNode> = {};
  const allNodeIds = new Set<string>();
  const nonRootNodes = new Set<string>();

  // Memoized node constructor
  const getNode = (
    id: string,
    type: NodeType,
    status: NodeStatus = 'UNKNOWN',
  ): GraphNode => {
    if (nodes[id]) return nodes[id];
    nodes[id] = {
      id,
      type,
      label: id,
      status: status,
      children: new Set(),
      parents: new Set(),
    };
    allNodeIds.add(id);
    return nodes[id];
  };

  // Initialize nodes for all checks and stages.
  checks.forEach((cv) => {
    const checkId = cv.check?.identifier?.id;
    if (!checkId) return;
    const checkNode = getNode(
      checkId,
      'CHECK',
      checkStateToStatus(cv.check?.state),
    );
    checkNode.raw = cv;
  });

  stages.forEach((sv) => {
    const stageId = sv.stage?.identifier?.id;
    if (!stageId) return;
    const stageNode = getNode(stageId, 'STAGE');
    stageNode.status = stageStateToStatus(sv.stage?.state);
    stageNode.raw = sv;

    // Add assignments as edges from check to stage.
    // A check is a parent of a stage if it is assigned to that stage.
    sv.stage?.assignments?.forEach((assignment) => {
      const checkId = assignment.target?.id;
      if (checkId) {
        const checkNode = getNode(checkId, 'CHECK', 'UNKNOWN');
        checkNode.children.add(stageId);
        getNode(stageId, 'STAGE').parents.add(checkId);
        nonRootNodes.add(stageId);
      }
    });

    // Add dependencies between checks.
    const assignedCheckIds =
      sv.stage?.assignments
        ?.map((a) => a.target?.id)
        .filter((id): id is string => !!id) || [];
    if (assignedCheckIds.length > 0) {
      sv.stage?.dependencies?.edges?.forEach((edge) => {
        const dependencyCheckId = edge.target?.check?.id;
        if (dependencyCheckId) {
          getNode(dependencyCheckId, 'CHECK');
          assignedCheckIds.forEach((assignedCheckId) => {
            if (dependencyCheckId !== assignedCheckId) {
              getNode(dependencyCheckId, 'CHECK').children.add(assignedCheckId);
              getNode(assignedCheckId, 'CHECK').parents.add(dependencyCheckId);
              nonRootNodes.add(assignedCheckId);
            }
          });
        }
      });
    }
  });

  // Roots are shown at the top level.
  const roots = Array.from(allNodeIds).filter((id) => !nonRootNodes.has(id));
  roots.sort();
  return { nodes, roots };
}

// Calculates the total number of unique nodes in a subtree
export function subtreeSize(graph: Graph, startNodeId: string): number {
  const visited = new Set<string>();
  const stack = [startNodeId];
  while (stack.length > 0) {
    const nodeId = stack.pop()!;
    if (!visited.has(nodeId)) {
      visited.add(nodeId);
      const node = graph.nodes[nodeId];
      if (node) {
        node.children.forEach((childId) => stack.push(childId));
      }
    }
  }
  return visited.size;
}

export interface FlatTreeItem {
  id: string; // The graph node ID
  key: string; // Unique rendering key for this specific row appearance
  depth: number;
  isRepeated: boolean;
  isCycle: boolean;
  hasChildren: boolean;
  parentId: string | null;
  isMatch: boolean; // True if this node directly matched the current filter
}

export function transitiveDescendants(
  graph: Graph,
  startIds: string | string[],
): Set<string> {
  const descendants = new Set<string>();
  const stack = Array.isArray(startIds) ? [...startIds] : [startIds];
  const visited = new Set<string>(stack);

  while (stack.length > 0) {
    const id = stack.pop()!;
    const node = graph.nodes[id];
    if (node) {
      node.children.forEach((childId) => {
        if (!visited.has(childId)) {
          visited.add(childId);
          descendants.add(childId);
          stack.push(childId);
        }
      });
    }
  }
  return descendants;
}
