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

import { CheckView } from '../../proto/turboci/graph/orchestrator/v1/check_view.pb';
import { Dependencies } from '../../proto/turboci/graph/orchestrator/v1/dependencies.pb';
import { GraphView as TurboCIGraphView } from '../../proto/turboci/graph/orchestrator/v1/graph_view.pb';
import { StageView } from '../../proto/turboci/graph/orchestrator/v1/stage_view.pb';

import {
  CheckResultStatus,
  getCheckLabel,
  getCheckResultStatus,
} from './check_utils';

const dagreGraph = new dagre.graphlib.Graph();
dagreGraph.setDefaultEdgeLabel(() => ({}));

// We must explicit set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
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

const ASSIGNMENT_EDGE_STYLE: Partial<Edge> = {
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: '#B0BEC5',
  },
  style: { strokeWidth: 1, stroke: '#abd3e7ff', strokeDasharray: '5 3' },
  zIndex: 0,
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
    bg: 'var(--light-background-color-1)',
    border: 'var(--light-background-color-4)',
    text: 'var(--default-text-color)',
  },
  checkSuccess: {
    bg: 'var(--success-bg-color)',
    border: 'var(--success-color)',
    text: 'var(--default-text-color)',
  },
  checkFailure: {
    bg: 'var(--failure-bg-color)',
    border: 'var(--failure-color)',
    text: 'var(--default-text-color)',
  },
  stage: {
    bg: 'var(--scheduled-bg-color)',
    border: 'var(--scheduled-color)',
    text: 'var(--default-text-color)',
  },
  collapsedGroup: {
    bg: 'var(--light-background-color-1)',
    border: 'var(--light-background-color-4)',
    text: 'var(--default-text-color)',
  },
};

type NodeColors = { bg: string; border: string; text: string };

const NODE_STYLES = {
  // Standalone Check
  check: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderTop: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: createCssBorder(colors.border),
    borderLeft: createCssBorder(colors.border),
    borderRadius: '4px',
  }),
  // Check attached below stage(s)
  checkGrouped: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    // Visual connector to stage above. Uses stage border color.
    borderTop: createCssBorder(COLORS.stage.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: createCssBorder(colors.border),
    borderLeft: createCssBorder(colors.border),
    borderRadius: '0 0 4px 4px', // Rounded bottom only
    // Ensure top border overlays stage bottom border area
    zIndex: 1,
    // Pull up by border width to overlap exactly
    marginTop: `-${GRAPH_CONFIG.borderWidth}px`,
  }),
  // Standalone Stage
  stage: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderTop: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: createCssBorder(colors.border),
    borderLeft: createCssBorder(colors.border),
    borderRadius: '4px',
    fontWeight: 'bold',
  }),
  // Single Stage attached above a Check
  stageGrouped: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderTop: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: 'none', // Overlapped by check
    borderLeft: createCssBorder(colors.border),
    borderRadius: '4px 4px 0 0',
    fontWeight: 'bold',
  }),
  // Top stage in a multi-stage stack
  stageGroupedTop: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderTop: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: 'none', // Overlapped by next node
    borderLeft: createCssBorder(colors.border),
    borderRadius: '4px 4px 0 0',
    fontWeight: 'bold',
    zIndex: 2, // On top of node below
  }),
  // Middle stage in a multi-stage stack
  stageGroupedMiddle: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderLeft: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderTop: 'none', // Overlapped by node above
    borderBottom: 'none', // Overlapped by node below
    borderRadius: '0',
    fontWeight: 'bold',
    zIndex: 1,
  }),
  // Bottom stage in a multi-stage stack (immediately above check)
  stageGroupedBottom: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderLeft: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderTop: 'none', // Overlapped by node above
    borderBottom: 'none', // Overlapped by check border
    borderRadius: '0',
    fontWeight: 'bold',
  }),
  // Style for a collapsed group of similar nodes
  collapsedGroup: (colors: NodeColors) => ({
    ...BASE_NODE_STYLE,
    background: colors.bg,
    color: colors.text,
    borderTop: createCssBorder(colors.border),
    borderRight: createCssBorder(colors.border),
    borderBottom: createCssBorder(colors.border),
    borderLeft: createCssBorder(colors.border),
    borderRadius: '4px',
    fontWeight: 'bold',
    justifyContent: 'center',
  }),
};

// Effective height for stacking (height - 1px border overlap)
// Assumes box-sizing: border-box in CSS.
const EFFECTIVE_HEIGHT = GRAPH_CONFIG.nodeHeight - GRAPH_CONFIG.borderWidth;

// The maximum length for a graph node label before it is truncated.
const LABEL_TRUNCATE_MAX_LENGTH = 60;

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

export interface GraphBuilderOptions {
  /** Whether to draw assignment edges for stages assigned to multiple checks. */
  showAssignmentEdges?: boolean;
  /**
   * Set of parent hashes that should be collapsed into a single node.
   * Nodes with the same dependencies/parents will have the same hash and will be collapsed together.
   */
  collapsedParentHashes?: Set<number>;
}

// Helper to generate full border string for a given color
function createCssBorder(color: string) {
  return `${BORDER_STYLE} ${color}`;
}

function getCheckColors(status: CheckResultStatus | undefined) {
  if (status === CheckResultStatus.SUCCESS) {
    return COLORS.checkSuccess;
  }
  if (status === CheckResultStatus.FAILURE) {
    return COLORS.checkFailure;
  }
  return COLORS.check;
}

function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = (hash << 5) - hash + char;
    hash = hash & hash;
  }
  return hash;
}

function truncateLabel(
  label: string,
  maxLength = LABEL_TRUNCATE_MAX_LENGTH,
): string {
  if (label.length <= maxLength) return label;
  return label.substring(0, maxLength) + '...';
}

function createCheckNode(checkView: CheckView, parentHash?: number): Node {
  const check = checkView.check!;
  const resultStatus = getCheckResultStatus(checkView);
  const colors = getCheckColors(resultStatus);
  return {
    id: check.identifier!.id!,
    data: {
      label: truncateLabel(getCheckLabel(checkView)),
      view: checkView,
      parentHash,
      resultStatus,
    },
    style: NODE_STYLES.check(colors),
    ...COMMON_NODE_PROPERTIES,
  };
}

function createStageNode(stageView: StageView): Node {
  const stage = stageView.stage!;
  return {
    id: stage.identifier!.id!,
    data: {
      label: truncateLabel(`Stage: ${stage.identifier!.id}`),
      view: stageView,
    },
    style: NODE_STYLES.stage(COLORS.stage),
    ...COMMON_NODE_PROPERTIES,
  };
}

function createCollapsedGroupNode(
  id: string,
  label: string,
  parentHash: number,
): Node {
  return {
    id,
    data: {
      label: truncateLabel(label),
      isCollapsed: true,
      parentHash,
    },
    style: NODE_STYLES.collapsedGroup(COLORS.collapsedGroup),
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
  // Maps a Node ID (Stage or Check) to the ID of the synthetic collapsed group
  // node it belongs to.
  private nodeToCollapsedIdMap: Map<string, string> = new Map();
  private allNodeIds: Set<string> = new Set();

  constructor(private readonly graphView: TurboCIGraphView) {}

  public build(options: GraphBuilderOptions = {}): {
    nodes: Node[];
    edges: Edge[];
  } {
    this.nodes = [];
    this.edges = [];
    this.assignmentGroups = [];
    this.nodeToGroupIdMap.clear();
    this.nodeToCollapsedIdMap.clear();
    this.allNodeIds.clear();

    this.createBaseNodes(options.collapsedParentHashes || new Set());
    this.identifyAssignmentGroups(options);
    this.createEdges();
    this.applyDagreLayout();

    return { nodes: this.nodes, edges: this.edges };
  }

  private createBaseNodes(collapsedParentHashes: Set<number>) {
    // 1. Place checks into groups by hashing their parent dependencies.
    const hashToGroup = new Map<number, string[]>();
    const nodeToHash = new Map<string, number>();

    Object.entries(this.graphView.checks).forEach(([checkId, checkView]) => {
      const deps = checkView.check?.dependencies?.edges || [];
      // Create a unique signature based on sorted parent IDs
      const parentIds = deps
        .map((e) => e.check?.identifier?.id || e.stage?.identifier?.id)
        .filter((id) => !!id)
        .sort();

      if (parentIds.length === 0) return;

      const hash = hashString(parentIds.join('|'));
      if (!hashToGroup.has(hash)) {
        hashToGroup.set(hash, []);
      }
      hashToGroup.get(hash)!.push(checkId);
      nodeToHash.set(checkId, hash);
    });

    // 2. Process groupings to decide what to collapse
    hashToGroup.forEach((checkIds, hash) => {
      // We only form a collapsible group if there is more than one item.
      if (checkIds.length <= 1) {
        return;
      }

      // If this hash is in the collapsed set, create a single group node
      if (collapsedParentHashes.has(hash)) {
        const collapsedId = `collapsed-group-${hash}`;

        // Map all constituents to this group ID so edges can be redirected
        checkIds.forEach((id) => {
          this.nodeToCollapsedIdMap.set(id, collapsedId);
        });

        // Add the synthetic node to the graph
        this.nodes.push(
          createCollapsedGroupNode(
            collapsedId,
            `Collapsed (${checkIds.length} items)`,
            hash,
          ),
        );
        this.allNodeIds.add(collapsedId);
      }
    });

    // 3. Create all Check nodes (skipping those that were collapsed)
    Object.entries(this.graphView.checks).forEach(([checkId, checkView]) => {
      if (this.nodeToCollapsedIdMap.has(checkId)) {
        return;
      }

      // Include the parent hash when creating the check to support collapsing
      // it (and its sibling nodes). However we only support this if it has > 1
      // siblings in its group, otherwise just use undefined to indicate that
      // it's not collapsible.
      const hash = nodeToHash.get(checkId);
      const groupSize = hash ? hashToGroup.get(hash)?.length || 0 : 0;
      const parentHash = groupSize > 1 ? hash : undefined;
      this.nodes.push(createCheckNode(checkView, parentHash));
      this.allNodeIds.add(checkId);
    });

    // 4. Create all Stage nodes
    Object.entries(this.graphView.stages).forEach(([stageId, stageView]) => {
      // If this stage is assigned to a check that is collapsed, we should
      // skip creating the stage node as well.
      if (!this.isAssignedToCollapsedGroup(stageView)) {
        this.nodes.push(createStageNode(stageView));
        this.allNodeIds.add(stageId);
      }
    });
  }

  private identifyAssignmentGroups(options: GraphBuilderOptions) {
    // key: checkId, value: stageIds assigned to check
    const checkAssignments = new Map<string, Set<string>>();

    Object.entries(this.graphView.stages).forEach(([stageId, sv]) => {
      if (!sv.stage) return;

      // Don't group stages that are assigned multiple different checks.
      // Instead, just create edges for them.
      const stageAssignedChecks = sv.stage.assignments
        .map((a) => a.target?.id)
        .filter((a) => a !== undefined);
      if (stageAssignedChecks.length !== 1) {
        if (options.showAssignmentEdges) {
          this.addAssignmentEdges(stageId, stageAssignedChecks);
        }
        return;
      }

      // At this point we know there is only a singular check assigned to this stage.
      const checkId = stageAssignedChecks[0];
      // If the check is collapsed, we skip creating assignment groups for it,
      // because the Stage node wasn't created anyway.
      if (this.nodeToCollapsedIdMap.has(checkId)) return;

      if (!checkAssignments.has(checkId)) {
        checkAssignments.set(checkId, new Set());
      }
      checkAssignments.get(checkId)!.add(stageId);
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

  /**
   * Normally assignments between a stage and a singular check are displayed
   * as a stacked group. But for stages with multiple assigned checks, we create
   * an edge.
   */
  private addAssignmentEdges(sourceNodeId: string, assignments: string[]) {
    // Filter out assignments to collapsed nodes to prevent edges pointing to nowhere.
    assignments.forEach((assignment) => {
      if (this.nodeToCollapsedIdMap.has(assignment)) return;
      if (!this.allNodeIds.has(assignment)) return;

      this.edges.push({
        id: `assignment-${sourceNodeId}-${assignment}`,
        source: sourceNodeId,
        target: assignment,
        zIndex: 1,
        // We don't want assignment edges to affect the Dagre layout
        // so mark the edge accordingly.
        data: { isAssignment: true },
        ...ASSIGNMENT_EDGE_STYLE,
      });
    });
  }

  private addDependencyEdges(
    sourceNodeId: string,
    deps: Dependencies | undefined,
  ) {
    if (!deps) return;

    // Remap source to collapsed group if applicable
    let actualSourceId = sourceNodeId;
    if (this.nodeToCollapsedIdMap.has(sourceNodeId)) {
      actualSourceId = this.nodeToCollapsedIdMap.get(sourceNodeId)!;
    }

    deps.edges.forEach((edge) => {
      let targetId = edge?.check?.identifier?.id || edge?.stage?.identifier?.id;

      if (!targetId) return;

      // Remap target to collapsed group if applicable
      if (this.nodeToCollapsedIdMap.has(targetId)) {
        targetId = this.nodeToCollapsedIdMap.get(targetId)!;
      }

      // Only create edge if target exists in graph
      if (this.allNodeIds.has(targetId)) {
        // Do not create visual edges between nodes in the same group
        const sourceGroup = this.nodeToGroupIdMap.get(actualSourceId);
        const targetGroup = this.nodeToGroupIdMap.get(targetId);

        if (sourceGroup && targetGroup && sourceGroup === targetGroup) {
          return;
        }

        const edgeId = `dep-${targetId}-${actualSourceId}`;
        // Check if we already added this edge (deduplication for collapsed nodes)
        if (this.edges.some((e) => e.id === edgeId)) return;

        // Edge goes from Dependency (target) -> Dependent (source)
        this.edges.push({
          id: edgeId,
          source: targetId,
          target: actualSourceId,
          zIndex: 1,
          ...DEPENDENCY_EDGE_STYLE,
        });
      }
    });
  }

  private createEdges() {
    Object.entries(this.graphView.stages).forEach(([stageId, sv]) => {
      // Don't process edges for stages that were hidden because they
      // were assigned to a collapsed check.
      if (this.isAssignedToCollapsedGroup(sv)) return;

      this.addDependencyEdges(stageId, sv.stage!.dependencies);
    });

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
      // Skip assignment edges during layout calculation so they don't affect node positioning.
      if (edge.data?.isAssignment) return;

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

    // Don't overwrite style if already styled as collapsed group.
    if (node.data.isCollapsed) {
      return;
    }

    // Assuming ID conventions from FakeGraphGenerator: S_* are stages.
    // TODO - Make stage/check detection more robust
    if (node.id.startsWith('S')) {
      node.style = NODE_STYLES.stage(COLORS.stage);
    } else {
      const colors = getCheckColors(node.data.resultStatus);
      node.style = NODE_STYLES.check(colors);
    }
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
      const colors = getCheckColors(node.data.resultStatus);
      node.position = {
        x: baseX,
        y: baseY + numStages * EFFECTIVE_HEIGHT,
      };
      node.style = NODE_STYLES.checkGrouped(colors);
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
      return NODE_STYLES.stageGrouped(COLORS.stage);
    } else if (index === 0) {
      return NODE_STYLES.stageGroupedTop(COLORS.stage);
    } else if (index === totalStages - 1) {
      return NODE_STYLES.stageGroupedBottom(COLORS.stage);
    } else {
      return NODE_STYLES.stageGroupedMiddle(COLORS.stage);
    }
  }

  private isAssignedToCollapsedGroup(stageView: StageView): boolean {
    const assignedCheckIds =
      stageView.stage?.assignments
        ?.map((a) => a.target?.id)
        .filter((id): id is string => !!id) || [];

    return assignedCheckIds.some((cid) => this.nodeToCollapsedIdMap.has(cid));
  }
}
