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

import { useEffect, useMemo } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';
import { CheckState } from '@/proto/turboci/graph/orchestrator/v1/check_state.pb';

import { CheckKind } from '../../proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { FakeGraphGenerator } from '../fake_turboci_graph';
import {
  convertTurboCIGraphToNodesAndEdges,
  getLayoutedElements,
} from '../utils/graph_utils';

// Fake TurboCI data. To be replaced once TurboCI APIs are available.
const graphGenerator = new FakeGraphGenerator({
  workPlanIdStr: 'test-plan',
  checkIds: [
    'CheckA',
    'CheckB',
    'CheckC',
    'CheckD',
    'CheckE',
    'CheckF',
    'CheckG',
    'CheckH',
  ],
  stageIds: ['StageA', 'StageB', 'StageC'],
  dependencies: {
    CheckB: ['CheckA'],
    CheckC: ['CheckA'],
    CheckD: ['CheckA'],
    CheckE: ['CheckA'],
    CheckF: ['CheckA'],
    CheckG: ['CheckC', 'CheckD', 'CheckE'],
    CheckH: ['CheckF', 'CheckG'],
    StageB: ['StageA'],
    StageC: ['StageB'],
    CheckA: ['StageA'],
  },
  checkKinds: {
    CheckA: CheckKind.CHECK_KIND_SOURCE,
    CheckB: CheckKind.CHECK_KIND_BUILD,
    CheckC: CheckKind.CHECK_KIND_BUILD,
    CheckD: CheckKind.CHECK_KIND_BUILD,
    CheckE: CheckKind.CHECK_KIND_BUILD,
    CheckF: CheckKind.CHECK_KIND_BUILD,
    CheckG: CheckKind.CHECK_KIND_TEST,
    CheckH: CheckKind.CHECK_KIND_TEST,
  },
  checkEdits: [
    {
      stageId: 'StageA',
      checkId: 'CheckA',
      state: CheckState.CHECK_STATE_PLANNING,
    },
    {
      stageId: 'StageA',
      checkId: 'CheckA',
      state: CheckState.CHECK_STATE_PLANNED,
    },
    {
      stageId: 'StageA',
      checkId: 'CheckA',
      state: CheckState.CHECK_STATE_WAITING,
    },
    {
      stageId: 'StageA',
      checkId: 'CheckA',
      state: CheckState.CHECK_STATE_FINAL,
    },
    {
      stageId: 'StageB',
      checkId: 'CheckB',
      state: CheckState.CHECK_STATE_FINAL,
    },
    {
      stageId: 'StageB',
      checkId: 'CheckC',
      state: CheckState.CHECK_STATE_FINAL,
    },
    {
      stageId: 'StageB',
      checkId: 'CheckD',
      state: CheckState.CHECK_STATE_PLANNED,
    },
    {
      stageId: 'StageB',
      checkId: 'CheckE',
      state: CheckState.CHECK_STATE_WAITING,
    },
    {
      stageId: 'StageB',
      checkId: 'CheckF',
      state: CheckState.CHECK_STATE_WAITING,
    },
    {
      stageId: 'StageC',
      checkId: 'CheckG',
      state: CheckState.CHECK_STATE_PLANNING,
    },
    {
      stageId: 'StageC',
      checkId: 'CheckH',
      state: CheckState.CHECK_STATE_PLANNING,
    },
  ],
});
const turboCiGraph = graphGenerator.generate();

function GraphView() {
  useDeclareTabId('graph');

  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);

  const { layoutedNodes, layoutedEdges } = useMemo(() => {
    // Convert TurboCI Graph to list of nodes and edges that React Flow understands.
    const { nodes: initialNodes, edges: initialEdges } =
      convertTurboCIGraphToNodesAndEdges(turboCiGraph);
    // Use Dagre to layout and position nodes according to their edges.
    const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(
      initialNodes,
      initialEdges,
    );
    return { layoutedNodes, layoutedEdges };
  }, []);

  useEffect(() => {
    setNodes(layoutedNodes);
    setEdges(layoutedEdges);
  }, [layoutedNodes, layoutedEdges]);

  return (
    <div style={{ height: '80vh' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}

export { GraphView as Component };
