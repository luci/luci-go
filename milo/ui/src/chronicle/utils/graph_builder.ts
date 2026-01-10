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
import { Edge, MarkerType, Node, Position } from '@xyflow/react';
import { CSSProperties } from 'react';

import { CheckKind } from '../../proto/turboci/graph/orchestrator/v1/check_kind.pb';
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

export type ChronicleNodeData = {
  label: string;
  view?: CheckView | StageView;
  groupId?: number;
  resultStatus?: CheckResultStatus;
  isCollapsed?: boolean;
  isGrouped?: boolean;
};

export type ChronicleNode = Node<ChronicleNodeData>;

export enum GroupMode {
  EXPANDED = 'EXPANDED',
  COLLAPSE_SUCCESS_ONLY = 'COLLAPSE_SUCCESS_ONLY',
  COLLAPSE_ALL = 'COLLAPSE_ALL',
}

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

// We must explicit set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
const COMMON_NODE_PROPERTIES: Partial<ChronicleNode> &
  Pick<ChronicleNode, 'position'> = {
  position: { x: 0, y: 0 }, // Position will be calculated by Dagre
  sourcePosition: Position.Right,
  targetPosition: Position.Left,
  draggable: false,
  initialWidth: GRAPH_CONFIG.nodeWidth,
  initialHeight: GRAPH_CONFIG.nodeHeight,
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

const ASSIGNMENT_EDGE_STYLE_INVISIBLE: Partial<Edge> = {
  style: { strokeWidth: 0, pointerEvents: 'none' },
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
  checkMixed: {
    bg: 'var(--warning-bg-color)',
    border: 'var(--warning-color)',
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
   * Mapping of group IDs (nodes with same children/parent) to their grouping mode (collapsed/expanded)
   * If not specified, it defaults to COLLAPSE_SUCCESS_ONLY.
   */
  groupModes?: Map<number, GroupMode>;
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
  if (status === CheckResultStatus.MIXED) {
    return COLORS.checkMixed;
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

function createCheckNode(checkView: CheckView, groupId: number): ChronicleNode {
  const check = checkView.check!;
  const resultStatus = getCheckResultStatus(checkView);
  const colors = getCheckColors(resultStatus);
  return {
    id: check.identifier!.id!,
    data: {
      label: truncateLabel(getCheckLabel(checkView)),
      view: checkView,
      groupId,
      resultStatus,
    },
    style: NODE_STYLES.check(colors),
    ...COMMON_NODE_PROPERTIES,
  };
}

function createStageNode(stageView: StageView): ChronicleNode {
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
  groupId: number,
  status?: CheckResultStatus,
): ChronicleNode {
  const colors = getCheckColors(status);
  return {
    id,
    data: {
      label: truncateLabel(label),
      isCollapsed: true,
      groupId,
      resultStatus: status,
    },
    style: NODE_STYLES.collapsedGroup(colors),
    ...COMMON_NODE_PROPERTIES,
  };
}

/**
 * Analyzes the graph to identify groups of checks based on topology.
 *
 * Checks are grouped together if they have identical set of parents and children.
 *
 * @param graphView The TurboCI GraphView to analyze.
 * @returns Lookup maps for topology groups.
 */
export function getTopologyGroups(graphView: TurboCIGraphView): {
  groupIdToChecks: Map<number, CheckView[]>;
  nodeToGroupId: Map<string, number>;
  parentToGroupIds: Map<string, number[]>;
} {
  const groupIdToChecks = new Map<number, CheckView[]>();
  const nodeToGroupId = new Map<string, number>();
  const parentToGroupIds = new Map<string, number[]>();

  const parentToChildren = new Map<string, string[]>();

  // Populate parentToChildren map
  Object.values(graphView.checks).forEach((cv) => {
    const childId = cv.check?.identifier?.id;
    if (childId) {
      cv.check?.dependencies?.edges?.forEach((edge) => {
        const parentId =
          edge.check?.identifier?.id || edge.stage?.identifier?.id;
        if (parentId) {
          if (!parentToChildren.has(parentId)) {
            parentToChildren.set(parentId, []);
          }
          parentToChildren.get(parentId)!.push(childId);
        }
      });
    }
  });

  // Group checks by their parents and children
  Object.entries(graphView.checks).forEach(([checkId, checkView]) => {
    const deps = checkView.check?.dependencies?.edges || [];
    const parentIds = deps
      .map((e) => e.check?.identifier?.id || e.stage?.identifier?.id)
      .filter((id): id is string => !!id)
      .sort();

    const childIds = (parentToChildren.get(checkId) || []).sort();

    // Create a unique signature based on sorted parent and child IDs.
    const signature = `P:${parentIds.join('|')};C:${childIds.join('|')}`;
    const hash = hashString(signature);

    if (!groupIdToChecks.has(hash)) {
      groupIdToChecks.set(hash, []);
    }
    groupIdToChecks.get(hash)!.push(checkView);
    nodeToGroupId.set(checkId, hash);
  });

  // Create mapping between parents and their collapsible child group IDs.
  for (const [hash, checks] of groupIdToChecks.entries()) {
    // Filter out groups with only one or zero items
    if (checks.length > 1) {
      // We only need to check one node in the group because they all share parents.
      const exampleCheck = checks[0];
      const deps = exampleCheck.check?.dependencies?.edges || [];
      deps.forEach((edge) => {
        const parentId =
          edge.check?.identifier?.id || edge.stage?.identifier?.id;
        if (parentId) {
          if (!parentToGroupIds.has(parentId)) {
            parentToGroupIds.set(parentId, []);
          }
          const groups = parentToGroupIds.get(parentId)!;
          if (!groups.includes(hash)) {
            groups.push(hash);
          }
        }
      });
    }
  }

  return { groupIdToChecks, nodeToGroupId, parentToGroupIds };
}

/**
 * Builds the React Flow graph from the TurboCI GraphView.
 * Groups stages to their assigned checks and positions them together.
 */
export class TurboCIGraphBuilder {
  private nodes: ChronicleNode[] = [];
  private edges: Edge[] = [];
  private edgeIds: Set<string> = new Set();
  private assignmentGroups: CheckAssignmentGroup[] = [];
  // Maps a Node ID (Stage or Check) to the ID of the group it belongs to.
  // We use the check ID as the ID of the group.
  private nodeToGroupIdMap: Map<string, string> = new Map();
  // Maps a Check ID to the ID of the visual node representing it.
  private checkIdToVisualNodeIdMap: Map<string, string> = new Map();
  private allNodeIds: Set<string> = new Set();

  constructor(private readonly graphView: TurboCIGraphView) {}

  public build(options: GraphBuilderOptions = {}): {
    nodes: ChronicleNode[];
    edges: Edge[];
  } {
    this.nodes = [];
    this.edges = [];
    this.edgeIds.clear();
    this.assignmentGroups = [];
    this.nodeToGroupIdMap.clear();
    this.checkIdToVisualNodeIdMap.clear();
    this.allNodeIds.clear();

    this.createBaseNodes(options.groupModes || new Map());
    this.identifyAssignmentGroups(options);
    this.createEdges();
    this.applyDagreLayout();

    return { nodes: this.nodes, edges: this.edges };
  }

  private createBaseNodes(groupModes: Map<number, GroupMode>) {
    // 1. Place checks into groups by hashing their parent and child dependencies
    const { groupIdToChecks } = getTopologyGroups(this.graphView);

    // 2. Process groupings to decide what to collapse
    groupIdToChecks.forEach((checks, groupId) => {
      // Default to COLLAPSE_SUCCESS_ONLY if not specified.
      const mode = groupModes.get(groupId) || GroupMode.COLLAPSE_SUCCESS_ONLY;

      if (mode === GroupMode.COLLAPSE_ALL && checks.length > 1) {
        this.createCollapsedAllNode(groupId, checks);
      } else if (mode === GroupMode.COLLAPSE_SUCCESS_ONLY) {
        // createSuccessOnlyNodes will create individual nodes for failed checks.
        this.createSuccessOnlyNodes(groupId, checks);
      } else {
        this.createIndividualNodes(groupId, checks);
      }
    });

    // 3. Create Stage Nodes
    Object.entries(this.graphView.stages).forEach(([stageId, stageView]) => {
      // Stages hidden if executing a collapsed check.
      if (this.shouldHideStage(stageView)) {
        return;
      }
      this.nodes.push(createStageNode(stageView));
      this.allNodeIds.add(stageId);
    });
  }

  private shouldHideStage(stageView: StageView): boolean {
    const assignedCheckIds =
      stageView.stage?.assignments
        ?.map((a) => a.target?.id)
        .filter((id): id is string => !!id) || [];

    if (assignedCheckIds.length === 0) return false;

    // Hide if all assigned checks are collapsed into a group.
    return assignedCheckIds.every((checkId) => {
      const visualId = this.checkIdToVisualNodeIdMap.get(checkId);
      return visualId && visualId.startsWith('collapsed-group-');
    });
  }

  private createCollapsedAllNode(groupId: number, checks: CheckView[]) {
    const nodeId = `collapsed-group-${groupId}`;
    // Map all constituent checks to this group node ID
    checks.forEach((c) =>
      this.checkIdToVisualNodeIdMap.set(c.check!.identifier!.id!, nodeId),
    );

    let hasSuccess = false;
    let hasFailure = false;
    let successCount = 0;

    checks.forEach((c) => {
      const s = getCheckResultStatus(c);
      if (s === CheckResultStatus.SUCCESS) {
        hasSuccess = true;
        successCount++;
      } else if (s === CheckResultStatus.FAILURE) hasFailure = true;
    });

    let status = CheckResultStatus.UNKNOWN;
    if (hasSuccess && hasFailure) status = CheckResultStatus.MIXED;
    else if (hasSuccess && !hasFailure) status = CheckResultStatus.SUCCESS;
    else if (!hasSuccess && hasFailure) status = CheckResultStatus.FAILURE;

    let label = '';
    const kind = checks[0].check?.kind;
    const typeStr =
      kind === CheckKind.CHECK_KIND_BUILD
        ? 'builds'
        : kind === CheckKind.CHECK_KIND_TEST
          ? 'tests'
          : 'checks';

    if (status === CheckResultStatus.MIXED) {
      label = `${successCount}/${checks.length} succeeded ${typeStr}`;
    } else if (status === CheckResultStatus.SUCCESS) {
      label = `${checks.length} successful ${typeStr}`;
    } else if (status === CheckResultStatus.FAILURE) {
      label = `${checks.length} failed ${typeStr}`;
    } else {
      label = `${checks.length} ${typeStr}`;
    }

    this.nodes.push(createCollapsedGroupNode(nodeId, label, groupId, status));
    this.allNodeIds.add(nodeId);
  }

  private createSuccessOnlyNodes(groupId: number, checks: CheckView[]) {
    const successes: CheckView[] = [];
    const failures: CheckView[] = [];

    checks.forEach((c) => {
      if (getCheckResultStatus(c) === CheckResultStatus.SUCCESS) {
        successes.push(c);
      } else {
        failures.push(c);
      }
    });

    this.createIndividualNodes(groupId, failures);

    if (successes.length > 1) {
      const nodeId = `collapsed-group-${groupId}-success`;
      successes.forEach((c) =>
        this.checkIdToVisualNodeIdMap.set(c.check!.identifier!.id!, nodeId),
      );

      const kind = successes[0].check?.kind;
      const typeStr =
        kind === CheckKind.CHECK_KIND_BUILD
          ? 'builds'
          : kind === CheckKind.CHECK_KIND_TEST
            ? 'tests'
            : 'checks';
      const label = `${successes.length} successful ${typeStr}`;

      this.nodes.push(
        createCollapsedGroupNode(
          nodeId,
          label,
          groupId,
          CheckResultStatus.SUCCESS,
        ),
      );
      this.allNodeIds.add(nodeId);
    } else {
      this.createIndividualNodes(groupId, successes);
    }
  }

  private createIndividualNodes(groupId: number, checks: CheckView[]) {
    checks.forEach((c) => {
      const checkId = c.check!.identifier!.id!;
      this.checkIdToVisualNodeIdMap.set(checkId, checkId);
      this.nodes.push(createCheckNode(c, groupId));
      this.allNodeIds.add(checkId);
    });
  }

  private identifyAssignmentGroups(options: GraphBuilderOptions) {
    // key: checkId, value: stageIds assigned to check
    const checkAssignments = new Map<string, Set<string>>();

    Object.entries(this.graphView.stages).forEach(([stageId, sv]) => {
      if (this.shouldHideStage(sv)) {
        return;
      }

      if (!sv.stage) return;

      // Don't group stages that are assigned multiple different checks.
      // Instead, just create edges for them.
      const stageAssignedChecks = sv.stage.assignments
        .map((a) => a.target?.id)
        .filter((a) => a !== undefined);
      if (stageAssignedChecks.length !== 1) {
        this.addAssignmentEdges(
          stageId,
          stageAssignedChecks,
          !!options.showAssignmentEdges,
        );
        return;
      }
      // At this point we know there is only a singular check assigned to this stage.
      const checkId = stageAssignedChecks[0];
      // If the check is collapsed, we skip creating assignment groups for it,
      // because the Stage node wasn't created anyway.
      const visualTargetId = this.checkIdToVisualNodeIdMap.get(checkId);
      if (!visualTargetId || !this.allNodeIds.has(visualTargetId)) return;

      if (!checkAssignments.has(visualTargetId)) {
        checkAssignments.set(visualTargetId, new Set());
      }
      checkAssignments.get(visualTargetId)!.add(stageId);
    });

    // Create assignment groups
    checkAssignments.forEach((stageIdSet, visualCheckId) => {
      if (stageIdSet.size > 0) {
        // Sort stages for deterministic layout order within stack
        const sortedStageIds = Array.from(stageIdSet).sort();
        this.assignmentGroups.push({
          checkId: visualCheckId,
          stageIds: sortedStageIds,
        });
        // Map nodes to this group
        this.nodeToGroupIdMap.set(visualCheckId, visualCheckId);
        sortedStageIds.forEach((sId) =>
          this.nodeToGroupIdMap.set(sId, visualCheckId),
        );
      }
    });
  }

  /**
   * Normally assignments between a stage and a singular check are displayed
   * as a stacked group. But for stages with multiple assigned checks, we create
   * an edge.
   */
  private addAssignmentEdges(
    sourceNodeId: string,
    assignments: string[],
    isVisible: boolean,
  ) {
    assignments.forEach((assignment) => {
      let targetId = assignment;
      // Remap target to collapsed group if applicable
      if (this.checkIdToVisualNodeIdMap.has(assignment)) {
        targetId = this.checkIdToVisualNodeIdMap.get(assignment)!;
      }

      if (!this.allNodeIds.has(targetId)) return;

      const edgeId = `assignment-${sourceNodeId}-${targetId}`;
      if (this.edgeIds.has(edgeId)) return;

      this.edgeIds.add(edgeId);

      // Make the edge invisible but still present for layout.
      const style = isVisible
        ? ASSIGNMENT_EDGE_STYLE
        : ASSIGNMENT_EDGE_STYLE_INVISIBLE;

      this.edges.push({
        id: edgeId,
        source: sourceNodeId,
        target: targetId,
        zIndex: 1,
        data: { isAssignment: true },
        ...style,
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
    if (this.checkIdToVisualNodeIdMap.has(sourceNodeId)) {
      actualSourceId = this.checkIdToVisualNodeIdMap.get(sourceNodeId)!;
    }

    deps.edges.forEach((edge) => {
      let targetId = edge?.check?.identifier?.id || edge?.stage?.identifier?.id;

      if (!targetId) return;

      // Remap target to collapsed group if applicable
      if (this.checkIdToVisualNodeIdMap.has(targetId)) {
        targetId = this.checkIdToVisualNodeIdMap.get(targetId)!;
      }

      // Only create edge if target exists in graph
      if (
        targetId &&
        this.allNodeIds.has(targetId) &&
        this.allNodeIds.has(sourceNodeId)
      ) {
        // Do not create visual edges between nodes in the same group
        const sourceGroup = this.nodeToGroupIdMap.get(actualSourceId);
        const targetGroup = this.nodeToGroupIdMap.get(targetId);

        if (sourceGroup && targetGroup && sourceGroup === targetGroup) {
          return;
        }

        const edgeId = `dep-${targetId}-${actualSourceId}`;
        // Check if we already added this edge (deduplication for collapsed nodes)
        if (this.edgeIds.has(edgeId)) return;

        this.edgeIds.add(edgeId);

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
      if (this.shouldHideStage(sv)) return;
      this.addDependencyEdges(stageId, sv.stage!.dependencies);
    });

    Object.entries(this.graphView.checks).forEach(([rawCheckId, cv]) => {
      const visualSourceId = this.checkIdToVisualNodeIdMap.get(rawCheckId);
      if (!visualSourceId) return;

      this.addDependencyEdges(visualSourceId, cv.check!.dependencies);
    });
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

    const nodesByDagreId = new Map<string, string>();

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

      dagreGraph.setNode(metaId, {
        width: GRAPH_CONFIG.nodeWidth,
        height: stackHeight,
        groupInfo,
      });

      // Map actual RF nodes to this dagre meta-node
      nodesByDagreId.set(checkId, metaId);
      stageIds.forEach((stageId) => {
        nodesByDagreId.set(stageId, metaId);
      });
    });

    // 2. Add standalone nodes to Dagre
    this.nodes.forEach((node) => {
      if (!nodesByDagreId.has(node.id)) {
        dagreGraph.setNode(node.id, {
          width: GRAPH_CONFIG.nodeWidth,
          height: GRAPH_CONFIG.nodeHeight,
        });
        nodesByDagreId.set(node.id, node.id);
      }
    });

    // 3. Add edges to Dagre (mapping original node IDs to group IDs if necessary)
    this.edges.forEach((edge) => {
      const dagreSource = nodesByDagreId.get(edge.source);
      const dagreTarget = nodesByDagreId.get(edge.target);
      // Avoid self-loops in Dagre if nodes belong to same group
      if (dagreSource && dagreTarget && dagreSource !== dagreTarget) {
        dagreGraph.setEdge(dagreSource, dagreTarget);
      }
    });

    // 4. Run Dagre Layout
    dagre.layout(dagreGraph);

    // 5. Apply calculated positions and styles to nodes
    this.nodes = this.nodes.map((node) => {
      const dagreId = nodesByDagreId.get(node.id);
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

  private layoutStandaloneNode(node: ChronicleNode, x: number, y: number) {
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
    node: ChronicleNode,
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
}
