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

import {
  Box,
  Checkbox,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Typography,
} from '@mui/material';
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';
import { useDebounce } from 'react-use';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  ReactFlowProvider,
  useReactFlow,
  Panel as ReactFlowPanel,
  Node,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';

import { WorkflowType } from '../fake_turboci_graph';
import { TurboCIGraphBuilder } from '../utils/graph_builder';

import { ChronicleContext } from './chronicle_context';
import { ContextMenu, ContextMenuState } from './context_menu';
import { InspectorPanel } from './inspector_panel/inspector_panel';

// We must explicit set all top/right/bottom/left border properties here instead
// of just setting "border" because React does not work well when mixing shorthand
// and non-shorthand CSS properties.
const HIGHLIGHTED_NODE_STYLE = {
  borderTop: '2px solid #007bff',
  borderRight: '2px solid #007bff',
  borderBottom: '2px solid #007bff',
  borderLeft: '2px solid #007bff',
  boxShadow: '0 0 10px #007bff',
};

const HIGHLIGHTED_EDGE_STYLE = {
  stroke: '#007bff',
  strokeWidth: 3,
};

function Graph() {
  const { graph, workflowType, setWorkflowType } = useContext(ChronicleContext);
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const { fitView } = useReactFlow();
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');
  const [showAssignmentEdges, setShowAssignmentEdges] = useState(false);
  const [autoFitSelection, setAutoFitSelection] = useState(true);
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>(
    undefined,
  );
  const [collapsedParentHashes, setCollapsedParentHashes] = useState<
    Set<number>
  >(new Set());
  const [contextMenuState, setContextMenuState] = useState<
    ContextMenuState | undefined
  >(undefined);

  const { layoutedNodes, layoutedEdges } = useMemo(() => {
    if (!graph) return { layoutedNodes: [], layoutedEdges: [] };

    // Convert TurboCI Graph to list of nodes and edges that React Flow understands.
    const { nodes, edges } = new TurboCIGraphBuilder(graph).build({
      showAssignmentEdges,
      collapsedParentHashes,
    });
    return { layoutedNodes: nodes, layoutedEdges: edges };
  }, [graph, showAssignmentEdges, collapsedParentHashes]);

  useDebounce(
    () => {
      setDebouncedSearchQuery(searchQuery);
    },
    300,
    [searchQuery],
  );

  // Unified effect for handling graph highlighting (both search and selection).
  // Selection takes precedence over search.
  useEffect(() => {
    let nextNodes = layoutedNodes;
    let nextEdges = layoutedEdges;
    let nodesToFit: string[] = [];

    if (selectedNodeId) {
      const relatedNodeIds = new Set<string>([selectedNodeId]);
      const relatedEdgeIds = new Set<string>();

      // Find immediate neighbors and connecting edges
      layoutedEdges.forEach((edge) => {
        if (edge.source === selectedNodeId) {
          relatedNodeIds.add(edge.target);
          relatedEdgeIds.add(edge.id);
        } else if (edge.target === selectedNodeId) {
          relatedNodeIds.add(edge.source);
          relatedEdgeIds.add(edge.id);
        }
      });

      // Highlight nodes
      nextNodes = nextNodes.map((node) => {
        if (relatedNodeIds.has(node.id)) {
          return {
            ...node,
            style: { ...node.style, ...HIGHLIGHTED_NODE_STYLE },
          };
        }
        return node;
      });

      // Highlight edges
      nextEdges = nextEdges.map((edge) => {
        if (relatedEdgeIds.has(edge.id)) {
          return {
            ...edge,
            style: { ...edge.style, ...HIGHLIGHTED_EDGE_STYLE },
            zIndex: 10, // Bring highlighted edges to front
          };
        }
        return edge;
      });

      // Only autofit on selection if the option is enabled
      if (autoFitSelection) {
        nodesToFit = Array.from(relatedNodeIds);
      }
    } else if (debouncedSearchQuery) {
      nextNodes = nextNodes.map((node) => {
        const nodeMatch = node.data.label
          ?.toLocaleLowerCase()
          .includes(debouncedSearchQuery.toLocaleLowerCase());

        if (nodeMatch) {
          nodesToFit.push(node.id);
          return {
            ...node,
            style: { ...node.style, ...HIGHLIGHTED_NODE_STYLE },
          };
        }
        return node;
      });
    }

    setNodes(nextNodes);
    setEdges(nextEdges);

    if (nodesToFit.length > 0) {
      fitView({
        nodes: nodesToFit.map((id) => ({ id })),
        duration: 500,
      });
    }
  }, [
    layoutedNodes,
    layoutedEdges,
    selectedNodeId,
    debouncedSearchQuery,
    setNodes,
    setEdges,
    fitView,
    autoFitSelection,
  ]);

  // Use useCallback even with no dependencies to prevent React creating a new
  // function reference on every render.
  // https://reactflow.dev/learn/advanced-use/performance#memoize-functions
  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNodeId(node.id);
    setContextMenuState(undefined);
  }, []);

  const onNodeContextMenu = useCallback(
    (event: React.MouseEvent, node: Node) => {
      event.preventDefault();
      if (node.data.parentHash) {
        setContextMenuState({
          mouseX: event.clientX - 2,
          mouseY: event.clientY - 4,
          node,
        });
      } else {
        setContextMenuState(undefined);
      }
    },
    [],
  );

  const onPaneClick = useCallback(() => {
    setSelectedNodeId(undefined);
    setContextMenuState(undefined);
  }, []);

  const onInspectorClose = useCallback(() => {
    setSelectedNodeId(undefined);
  }, []);

  const handleContextMenuClose = useCallback(() => {
    setContextMenuState(undefined);
  }, []);

  const handleCollapseSimilar = useCallback((parentHash: number) => {
    setCollapsedParentHashes((prev) => {
      const next = new Set(prev);
      next.add(parentHash);
      return next;
    });
  }, []);

  const handleExpandGroup = useCallback((parentHash: number) => {
    setCollapsedParentHashes((prev) => {
      const next = new Set(prev);
      next.delete(parentHash);
      return next;
    });
  }, []);

  const selectedNode = useMemo(() => {
    return nodes.find((n) => n.id === selectedNodeId);
  }, [nodes, selectedNodeId]);

  return (
    <PanelGroup
      direction="horizontal"
      style={{ height: '100%', width: '100%' }}
    >
      <Panel minSize={50}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={onNodeClick}
          onNodeContextMenu={onNodeContextMenu}
          onPaneClick={onPaneClick}
          fitView
          panOnScroll
          minZoom={0.1}
        >
          <Background />
          <Controls />
          <MiniMap pannable zoomable />
          <ReactFlowPanel position="top-left">
            <Paper
              elevation={2}
              sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: 0,
                p: 1,
                borderRadius: 1,
              }}
            >
              <input
                type="text"
                placeholder="Search nodes..."
                value={searchQuery}
                onChange={(e) => {
                  setSearchQuery(e.target.value);
                  // Clear selected node when searching
                  if (e.target.value) {
                    setSelectedNodeId(undefined);
                  }
                }}
                style={{
                  padding: '8px',
                  width: '200px',
                  marginBottom: '8px',
                }}
              />
              <FormControl fullWidth size="small" sx={{ mt: 2 }}>
                <InputLabel id="workflow-type-select-label">
                  Workflow Type
                </InputLabel>
                <Select
                  labelId="workflow-type-select-label"
                  id="workflow-type-select"
                  value={workflowType}
                  label="Workflow Type"
                  onChange={(e) =>
                    setWorkflowType(e.target.value as WorkflowType)
                  }
                >
                  <MenuItem value={WorkflowType.ANDROID}>Android</MenuItem>
                  <MenuItem value={WorkflowType.BROWSER}>Browser</MenuItem>
                  <MenuItem value={WorkflowType.BROWSER_FUTURE}>
                    Browser Future
                  </MenuItem>
                </Select>
              </FormControl>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={showAssignmentEdges}
                    onChange={(e) => setShowAssignmentEdges(e.target.checked)}
                  />
                }
                label={
                  <Typography variant="body2">Show Assignment Edges</Typography>
                }
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={autoFitSelection}
                    onChange={(e) => setAutoFitSelection(e.target.checked)}
                  />
                }
                label={
                  <Typography variant="body2">Fit View on Selection</Typography>
                }
              />
            </Paper>
          </ReactFlowPanel>
        </ReactFlow>
        <ContextMenu
          contextMenuState={contextMenuState}
          onClose={handleContextMenuClose}
          onCollapseSimilar={handleCollapseSimilar}
          onExpandGroup={handleExpandGroup}
        />
      </Panel>
      {selectedNodeId && (
        <>
          <PanelResizeHandle>
            <Box
              sx={{
                width: '8px',
                height: '100%',
                cursor: 'col-resize',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
              }}
            >
              <Box sx={{ width: '2px', height: '24px', bgcolor: 'divider' }} />
            </Box>
          </PanelResizeHandle>
          <Panel defaultSize={30} minSize={20}>
            <InspectorPanel
              nodeId={selectedNodeId}
              viewData={selectedNode?.data?.view}
              onClose={onInspectorClose}
            />
          </Panel>
        </>
      )}
    </PanelGroup>
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
