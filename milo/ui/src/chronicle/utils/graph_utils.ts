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
import { Edge, Node, Position } from 'reactflow';
import 'reactflow/dist/style.css';

import { Check } from '../../proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckKind } from '../../proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { CheckView } from '../../proto/turboci/graph/orchestrator/v1/check_view.pb';
import { Edge as TurboCIEdge } from '../../proto/turboci/graph/orchestrator/v1/edge.pb';
import { GraphView as TurboCIGraphView } from '../../proto/turboci/graph/orchestrator/v1/graph_view.pb';
import { StageView } from '../../proto/turboci/graph/orchestrator/v1/stage_view.pb';

const dagreGraph = new dagre.graphlib.Graph();
dagreGraph.setDefaultEdgeLabel(() => ({}));

const nodeWidth = 180;
const nodeHeight = 36;

const checkNodeStyle = {
  background: '#c3beffff',
  color: '#333',
  border: '1px solid black',
};
const stageNodeStyle = {
  background: '#abffbdff',
  color: '#333',
  border: '1px solid black',
};
const commonNodeProperties = {
  position: { x: 0, y: 0 }, // Position will be calculated by Dagre
  sourcePosition: Position.Right,
  targetPosition: Position.Left,
};

/**
 * Uses Dagre to layout nodes in the graph.
 * React Flow provides graph visualization. Dagre calculates the positioning and provides
 * coordinates for nodes based on their edge dependencies.
 */
export function getLayoutedElements(nodes: Node[], edges: Edge[]) {
  // Parent nodes on left, children on right.
  dagreGraph.setGraph({ rankdir: 'LR' });

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: nodeWidth, height: nodeHeight });
  });

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  dagre.layout(dagreGraph);

  nodes.forEach((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    node.position = {
      x: nodeWithPosition.x - nodeWidth / 2,
      y: nodeWithPosition.y - nodeHeight / 2,
    };
  });

  return { nodes, edges };
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
    data: { label: getCheckNodeLabel(check) },
    style: checkNodeStyle,
    ...commonNodeProperties,
  };
}

function createStageNode(stageView: StageView): Node {
  const stage = stageView.stage!;
  return {
    id: stage.identifier!.id!,
    data: { label: `Stage: ${stage.identifier!.id}` },
    style: stageNodeStyle,
    ...commonNodeProperties,
  };
}

function createEdge(sourceId: string, depEdge: TurboCIEdge): Edge {
  const targetId = (depEdge.target!.check?.id || depEdge.target!.stage?.id)!;
  if (!targetId) {
    throw new Error(`Edge target has no ID: ${depEdge}`);
  }
  return {
    id: `e-${sourceId}-${targetId}`,
    source: targetId,
    target: sourceId,
    animated: true,
  };
}

export function convertTurboCIGraphToNodesAndEdges(graph: TurboCIGraphView) {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  // Create nodes for each stage
  graph.stages.forEach((stageView) => {
    if (
      !stageView ||
      !stageView.stage ||
      !stageView.stage.identifier ||
      !stageView.stage.identifier.id
    ) {
      throw new Error(`Invalid StageView: ${JSON.stringify(stageView)}`);
    }

    nodes.push(createStageNode(stageView));

    // Create edges for dependencies
    stageView.stage.dependencies.forEach((depGroup) => {
      depGroup.edges.forEach((edge) => {
        edges.push(createEdge(stageView.stage!.identifier!.id!, edge));
      });
    });
  });

  // Create nodes for each check
  graph.checks.forEach((checkView) => {
    if (
      !checkView ||
      !checkView.check ||
      !checkView.check.identifier ||
      !checkView.check.identifier.id
    ) {
      throw new Error(`Invalid CheckView: ${JSON.stringify(checkView)}`);
    }

    nodes.push(createCheckNode(checkView));

    // Create edges for dependencies
    checkView.check.dependencies.forEach((depGroup) => {
      depGroup.edges.forEach((edge) => {
        edges.push(createEdge(checkView.check!.identifier!.id!, edge));
      });
    });

    // TODO - Create edges for check edits
  });

  return { nodes, edges };
}
