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

import { useEffect, useMemo, useState } from 'react';
import { useDebounce } from 'react-use';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  ReactFlowProvider,
  useReactFlow,
  Panel,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';

import { FakeGraphGenerator } from '../fake_turboci_graph';
import { TurboCIGraphBuilder } from '../utils/graph_builder';

// Fake TurboCI data. To be replaced once TurboCI APIs are available.
const graphGenerator = new FakeGraphGenerator({
  workPlanIdStr: 'test-plan',
  numBuilds: 30,
  avgTestsPerBuild: 3,
  numSourceChanges: 3,
});
const turboCiGraph = graphGenerator.generate();

// We must explicit set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
const matchingNodeStyle = {
  borderTop: '2px solid #007bff',
  borderRight: '2px solid #007bff',
  borderBottom: '2px solid #007bff',
  borderLeft: '2px solid #007bff',
  boxShadow: '0 0 10px #007bff',
};

function Graph() {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const { fitView } = useReactFlow();
  const [searchQuery, setSearchQuery] = useState('');

  const { layoutedNodes, layoutedEdges } = useMemo(() => {
    // Convert TurboCI Graph to list of nodes and edges that React Flow understands.
    const { nodes, edges } = new TurboCIGraphBuilder(turboCiGraph).build();
    return { layoutedNodes: nodes, layoutedEdges: edges };
  }, []);

  useEffect(() => {
    setNodes(layoutedNodes);
    setEdges(layoutedEdges);
  }, [layoutedNodes, layoutedEdges]);

  useDebounce(
    () => {
      const matchedNodeIds: string[] = [];
      const searchFilteredNodes = layoutedNodes.map((node) => {
        const nodeMatch =
          searchQuery &&
          node.data.label
            ?.toLocaleLowerCase()
            .includes(searchQuery.toLocaleLowerCase());

        if (nodeMatch) {
          matchedNodeIds.push(node.id);
          return {
            ...node,
            style: {
              ...node.style,
              ...matchingNodeStyle,
            },
          };
        } else {
          return node;
        }
      });

      if (matchedNodeIds.length > 0) {
        fitView({
          nodes: matchedNodeIds.map((id) => ({ id })),
          duration: 500,
        });
      }

      setNodes(searchFilteredNodes);
    },
    300,
    [searchQuery],
  );

  return (
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
      <Panel position="top-left">
        <input
          type="text"
          placeholder="Search nodes..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          style={{ padding: '8px', width: '200px' }}
        />
      </Panel>
    </ReactFlow>
  );
}

// ReactFlowProvider must wrap the child component in order for the child component
// to utilize React Flow hooks.
function GraphView() {
  useDeclareTabId('graph');

  return (
    <div style={{ height: '80vh' }}>
      <ReactFlowProvider>
        <Graph />
      </ReactFlowProvider>
    </div>
  );
}

export { GraphView as Component };
