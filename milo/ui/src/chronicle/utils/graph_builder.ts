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

import dagre from '@dagrejs/dagre';
import { CSSProperties } from 'react';
import { Edge, MarkerType, Node, Position } from 'reactflow';
import 'reactflow/dist/style.css';

import { Check } from '../../proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckKind } from '../../proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { CheckView } from '../../proto/turboci/graph/orchestrator/v1/check_view.pb';
import { Dependencies } from '../../proto/turboci/graph/orchestrator/v1/dependencies.pb';
import { GraphView as TurboCIGraphView } from '../../proto/turboci/graph/orchestrator/v1/graph_view.pb';
import { StageView } from '../../proto/turboci/graph/orchestrator/v1/stage_view.pb';

const dagreGraph = new dagre.graphlib.Graph();
dagreGraph.setDefaultEdgeLabel(() => ({}));

// We must explicit set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
const CHECK_NODE_STYLE: CSSProperties = {
  background: '#c3beffff',
  color: '#333',
  borderTop: '1px solid black',
  borderRight: '1px solid black',
  borderBottom: '1px solid black',
  borderLeft: '1px solid black',
};
const STAGE_NODE_STYLE: CSSProperties = {
  background: '#abffbdff',
  color: '#333',
  borderTop: '1px solid black',
  borderRight: '1px solid black',
  borderBottom: '1px solid black',
  borderLeft: '1px solid black',
};
const COMMON_NODE_PROPERTIES: Partial<Node> & Pick<Node, 'position'> = {
  position: { x: 0, y: 0 }, // Position will be calculated by Dagre
  sourcePosition: Position.Right,
  targetPosition: Position.Left,
  draggable: false,
};

const DEPENDENCY_EDGE_STYLE: Partial<Edge> = {
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: '#B0BEC5',
  },
  style: { strokeWidth: 1.5, stroke: '#B0BEC5' },
};

const GRAPH_CONFIG = {
  nodeWidth: 240,
  nodeHeight: 32,
  // Border width around nodes
  borderWidth: 1,
  // Horizontal spacing between nodes
  rankSep: 250,
  // Vertical spacing between nodes
  nodeSep: 25,
};

// Common base styles for all nodes
const BASE_NODE_STYLE: CSSProperties = {
  fontSize: '12px',
  display: 'flex',
  alignItems: 'center',
  paddingLeft: '8px',
  boxSizing: 'border-box',
  width: GRAPH_CONFIG.nodeWidth,
  height: GRAPH_CONFIG.nodeHeight,
  margin: 0,
};

const BORDER_STYLE = `${GRAPH_CONFIG.borderWidth}px solid`;

const COLORS = {
  check: {
    bg: '#e3f2fd',
    border: '#90caf9',
    text: '#0d47a1',
  },
  stage: {
    bg: '#e8f5e9',
    border: '#81c784',
    text: '#1b5e20',
  },
};

const NODE_STYLES = {
  // Standalone Check
  check: {
    ...BASE_NODE_STYLE,
    background: COLORS.check.bg,
    color: COLORS.check.text,
    borderTop: createCssBorder(COLORS.check.border),
    borderRight: createCssBorder(COLORS.check.border),
    borderBottom: createCssBorder(COLORS.check.border),
    borderLeft: createCssBorder(COLORS.check.border),
    borderRadius: '4px',
  },
  // Check attached below stage(s)
  checkGrouped: {
    ...BASE_NODE_STYLE,
    background: COLORS.check.bg,
    color: COLORS.check.text,
    // Visual connector to stage above. Uses stage border color.
    borderTop: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.check.border),
    borderBottom: createCssBorder(COLORS.check.border),
    borderLeft: createCssBorder(COLORS.check.border),
    borderRadius: '0 0 4px 4px', // Rounded bottom only
    // Ensure top border overlays stage bottom border area
    zIndex: 1,
    // Pull up by border width to overlap exactly
    marginTop: `-${GRAPH_CONFIG.borderWidth}px`,
  },
  // Standalone Stage
  stage: {
    ...BASE_NODE_STYLE,
    background: COLORS.stage.bg,
    color: COLORS.stage.text,
    borderTop: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.stage.border),
    borderBottom: createCssBorder(COLORS.stage.border),
    borderLeft: createCssBorder(COLORS.stage.border),
    borderRadius: '4px',
    fontWeight: 'bold',
  },
  // Single Stage attached above a Check
  stageGrouped: {
    ...BASE_NODE_STYLE,
    background: COLORS.stage.bg,
    color: COLORS.stage.text,
    borderTop: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.stage.border),
    borderBottom: 'none', // Overlapped by check
    borderLeft: createCssBorder(COLORS.stage.border),
    borderRadius: '4px 4px 0 0',
    fontWeight: 'bold',
  },
  // Top stage in a multi-stage stack
  stageGroupedTop: {
    ...BASE_NODE_STYLE,
    background: COLORS.stage.bg,
    color: COLORS.stage.text,
    borderTop: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.stage.border),
    borderBottom: 'none', // Overlapped by next node
    borderLeft: createCssBorder(COLORS.stage.border),
    borderRadius: '4px 4px 0 0',
    fontWeight: 'bold',
    zIndex: 2, // On top of node below
  },
  // Middle stage in a multi-stage stack
  stageGroupedMiddle: {
    ...BASE_NODE_STYLE,
    background: COLORS.stage.bg,
    color: COLORS.stage.text,
    borderLeft: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.stage.border),
    borderTop: 'none', // Overlapped by node above
    borderBottom: 'none', // Overlapped by node below
    borderRadius: '0',
    fontWeight: 'bold',
    zIndex: 1,
  },
  // Bottom stage in a multi-stage stack (immediately above check)
  stageGroupedBottom: {
    ...BASE_NODE_STYLE,
    background: COLORS.stage.bg,
    color: COLORS.stage.text,
    borderLeft: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(COLORS.stage.border),
    borderTop: 'none', // Overlapped by node above
    borderBottom: 'none', // Overlapped by check border
    borderRadius: '0',
    fontWeight: 'bold',
  },
};

// Effective height for stacking (height - 1px border overlap)
// Assumes box-sizing: border-box in CSS.
const EFFECTIVE_HEIGHT = GRAPH_CONFIG.nodeHeight - GRAPH_CONFIG.borderWidth;

/**
 * A group consisting of a check and the stages assigned to them.
 */
interface CheckAssignmentGroup {
  checkId: string;
  stageIds: string[];
}

/** Contains information to render an assignment group node in Dagre */
interface DagreAssignmentGroupData {
  width: number;
  height: number;
  groupInfo: CheckAssignmentGroup;
}

// Helper to generate full border string for a given color
function createCssBorder(color: string) {
  return `${BORDER_STYLE} ${color}`;
}

// Helper function to get the human-readable label for a check node.
function getCheckNodeLabel(check: Check): string {
  const id = check.identifier!.id;
  switch (check.kind) {
    case CheckKind.CHECK_KIND_BUILD:
      return `Build Check: ${id}`;
    case CheckKind.CHECK_KIND_TEST:
      return `Test Check: ${id}`;
    case CheckKind.CHECK_KIND_SOURCE:
      return `Source Check: ${id}`;
    case CheckKind.CHECK_KIND_ANALYSIS:
      return `Analysis Check: ${id}`;
    case CheckKind.CHECK_KIND_UNKNOWN:
    default:
      return `Check: ${id}`;
  }
}

function createCheckNode(checkView: CheckView): Node {
  const check = checkView.check!;
  return {
    id: check.identifier!.id!,
    data: {
      label: getCheckNodeLabel(check),
      view: checkView,
    },
    style: CHECK_NODE_STYLE,
    ...COMMON_NODE_PROPERTIES,
  };
}

function createStageNode(stageView: StageView): Node {
  const stage = stageView.stage!;
  return {
    id: stage.identifier!.id!,
    data: {
      label: `Stage: ${stage.identifier!.id}`,
      view: stageView,
    },
    style: STAGE_NODE_STYLE,
    ...COMMON_NODE_PROPERTIES,
  };
}

/**
 * Builds the React Flow graph from the TurboCI GraphView.
 * Groups stages to their assigned checks and positions them together.
 */
export class TurboCIGraphBuilder {
  private nodes: Node[] = [];
  private edges: Edge[] = [];
  private assignmentGroups: CheckAssignmentGroup[] = [];
  // Maps a Node ID (Stage or Check) to the ID of the group it belongs to.
  // We use the check ID as the ID of the group.
  private nodeToGroupIdMap: Map<string, string> = new Map();
  private allNodeIds: Set<string> = new Set();

  constructor(private readonly graphView: TurboCIGraphView) {}

  public build(): { nodes: Node[]; edges: Edge[] } {
    this.nodes = [];
    this.edges = [];
    this.assignmentGroups = [];
    this.nodeToGroupIdMap.clear();

    this.createBaseNodes();
    this.identifyAssignmentGroups();
    this.createEdges();
    this.applyDagreLayout();

    return { nodes: this.nodes, edges: this.edges };
  }

  private createBaseNodes() {
    // Create nodes for each stage
    Object.entries(this.graphView.stages).forEach(([stageId, stageView]) => {
      this.nodes.push(createStageNode(stageView));
      this.allNodeIds.add(stageId);
    });

    // Create nodes for each check
    Object.entries(this.graphView.checks).forEach(([checkId, checkView]) => {
      this.nodes.push(createCheckNode(checkView));
      this.allNodeIds.add(checkId);
    });
  }

  private identifyAssignmentGroups() {
    // key: checkId, value: stageIds assigned to check
    const checkAssignments = new Map<string, Set<string>>();

    Object.entries(this.graphView.stages).forEach(([stageId, sv]) => {
      if (!sv.stage) return;

      // Ignore stages that are assigned multiple different checks.
      // TODO - Wire up edges to these stages
      const stageAssignedChecks = sv.stage.assignments.map((a) => a.target?.id);
      if (stageAssignedChecks.length !== 1) return;

      sv.stage.assignments.forEach((assignment) => {
        if (assignment.target?.id) {
          const checkId = assignment.target.id;
          if (!checkAssignments.has(checkId)) {
            checkAssignments.set(checkId, new Set());
          }
          checkAssignments.get(checkId)!.add(stageId);
        }
      });
    });

    // Create assignment groups
    checkAssignments.forEach((stageIdSet, checkId) => {
      if (stageIdSet.size > 0) {
        // Sort stages for deterministic layout order within stack
        const sortedStageIds = Array.from(stageIdSet).sort();
        this.assignmentGroups.push({ checkId, stageIds: sortedStageIds });

        // Map nodes to this group
        this.nodeToGroupIdMap.set(checkId, checkId);
        sortedStageIds.forEach((sId) =>
          this.nodeToGroupIdMap.set(sId, checkId),
        );
      }
    });
  }

  private addDependencyEdges(
    sourceNodeId: string,
    deps: Dependencies | undefined,
  ) {
    if (!deps) return;

    deps.edges.forEach((edge) => {
      const targetId = edge.target?.check?.id || edge.target?.stage?.id;

      // Only create edge if target exists in graph
      if (targetId && this.allNodeIds.has(targetId)) {
        // Do not create visual edges between nodes in the same group
        const sourceGroup = this.nodeToGroupIdMap.get(sourceNodeId);
        const targetGroup = this.nodeToGroupIdMap.get(targetId);

        if (sourceGroup && targetGroup && sourceGroup === targetGroup) {
          return;
        }

        // Edge goes from Dependency (target) -> Dependent (source)
        this.edges.push({
          id: `dep-${targetId}-${sourceNodeId}`,
          source: targetId,
          target: sourceNodeId,
          zIndex: 1,
          ...DEPENDENCY_EDGE_STYLE,
        });
      }
    });
  }

  private createEdges() {
    Object.entries(this.graphView.stages).forEach(([stageId, sv]) =>
      this.addDependencyEdges(stageId, sv.stage!.dependencies),
    );
    Object.entries(this.graphView.checks).forEach(([checkId, cv]) =>
      this.addDependencyEdges(checkId, cv.check!.dependencies),
    );
  }

  private applyDagreLayout() {
    const dagreGraph = new dagre.graphlib.Graph<DagreAssignmentGroupData>();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({
      rankdir: 'LR',
      nodesep: GRAPH_CONFIG.nodeSep,
      ranksep: GRAPH_CONFIG.rankSep,
      marginx: 20,
      marginy: 20,
    });

    // Maps node ID to its group ID.
    // If not in a group, the group ID is the node ID itself.
    const nodesByGroupId = new Map<string, string>();

    // 1. Create Dagre nodes for assignment groups
    this.assignmentGroups.forEach((groupInfo) => {
      const { checkId, stageIds } = groupInfo;
      // ID for the meta-node in Dagre
      const metaId = `group-${checkId}`;

      const totalNodesInStack = stageIds.length + 1; // Stages + Check

      // Calculate total height of the stack based on effective heights.
      // Base height + (N-1) overlaps.
      const stackHeight =
        GRAPH_CONFIG.nodeHeight + (totalNodesInStack - 1) * EFFECTIVE_HEIGHT;

      const nodeData: DagreAssignmentGroupData = {
        width: GRAPH_CONFIG.nodeWidth,
        height: stackHeight,
        groupInfo: groupInfo,
      };

      dagreGraph.setNode(metaId, nodeData);

      // Map actual RF nodes to this dagre meta-node
      nodesByGroupId.set(checkId, metaId);
      stageIds.forEach((stageId) => {
        nodesByGroupId.set(stageId, metaId);
      });
    });

    // 2. Add standalone nodes to Dagre
    this.nodes.forEach((node) => {
      if (!nodesByGroupId.has(node.id)) {
        dagreGraph.setNode(node.id, {
          width: GRAPH_CONFIG.nodeWidth,
          height: GRAPH_CONFIG.nodeHeight,
        });
        nodesByGroupId.set(node.id, node.id);
      }
    });

    // 3. Add edges to Dagre (mapping original node IDs to group IDs if necessary)
    this.edges.forEach((edge) => {
      const dagreSource = nodesByGroupId.get(edge.source);
      const dagreTarget = nodesByGroupId.get(edge.target);

      // Avoid self-loops in Dagre if nodes belong to same group
      if (dagreSource && dagreTarget && dagreSource !== dagreTarget) {
        dagreGraph.setEdge(dagreSource, dagreTarget);
      }
    });

    // 4. Run Dagre Layout
    dagre.layout(dagreGraph);

    // 5. Apply calculated positions and styles to nodes
    this.nodes = this.nodes.map((node) => {
      const dagreId = nodesByGroupId.get(node.id);
      if (!dagreId) return node; // Should not happen

      // dagreNode can be a simple node or one containing DagreAssignmentGroupData
      const dagreNode: dagre.Node<DagreAssignmentGroupData> =
        dagreGraph.node(dagreId);

      // Top-left of the node/meta-node bounding box
      const baseX = dagreNode.x - dagreNode.width / 2;
      const baseY = dagreNode.y - dagreNode.height / 2;

      if (dagreNode.groupInfo) {
        // Node belongs to a group; calculate position within stack.
        this.layoutGroupedNode(node, baseX, baseY, dagreNode.groupInfo);
      } else {
        // Standalone node
        this.layoutStandaloneNode(node, baseX, baseY);
      }

      return node;
    });
  }

  private layoutStandaloneNode(node: Node, x: number, y: number) {
    node.data = { ...node.data, isGrouped: false };
    node.position = { x, y };
    // Assuming ID conventions from FakeGraphGenerator: S_* are stages.
    // TODO - Make stage/check detection more robust
    node.style = node.id.startsWith('S')
      ? NODE_STYLES.stage
      : NODE_STYLES.check;
  }

  private layoutGroupedNode(
    node: Node,
    baseX: number,
    baseY: number,
    group: CheckAssignmentGroup,
  ) {
    node.data = { ...node.data, isGrouped: true };
    const { stageIds, checkId } = group;
    const numStages = stageIds.length;

    if (node.id === checkId) {
      // Check goes at the bottom of the stack
      // Y offset is N * EFFECTIVE_HEIGHT
      node.position = {
        x: baseX,
        y: baseY + numStages * EFFECTIVE_HEIGHT,
      };
      node.style = NODE_STYLES.checkGrouped;
    } else {
      // It's a stage
      const stageIndex = stageIds.indexOf(node.id);
      if (stageIndex !== -1) {
        node.position = {
          x: baseX,
          y: baseY + stageIndex * EFFECTIVE_HEIGHT,
        };
        node.style = this.getGroupedStageStyle(stageIndex, numStages);
      }
    }
  }

  private getGroupedStageStyle(index: number, totalStages: number) {
    if (totalStages === 1) {
      return NODE_STYLES.stageGrouped;
    } else if (index === 0) {
      return NODE_STYLES.stageGroupedTop;
    } else if (index === totalStages - 1) {
      return NODE_STYLES.stageGroupedBottom;
    } else {
      return NODE_STYLES.stageGroupedMiddle;
    }
  }
}
