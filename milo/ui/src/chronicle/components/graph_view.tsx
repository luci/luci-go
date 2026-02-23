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
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Typography,
  Snackbar,
  Alert,
  CircularProgress,
} from '@mui/material';
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
  FitViewOptions,
  Edge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';
import { useDebounce } from 'react-use';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';

import { WorkflowType } from '../fake_turboci_graph';
import { ChronicleNode, GroupMode } from '../utils/graph_builder';
// ?worker&url is special vite syntax to import a web worker script
// and retrieve the URL to the script.
// eslint-disable-next-line import/default
import graphWorkerUrl from '../utils/graph_worker?worker&url';

import { ChronicleContext } from './chronicle_context';
import { ContextMenu, ContextMenuState } from './context_menu';
import { useCollapsibleGroups } from './hooks/use_collapsible_groups';
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

let graphWorkerPolicy: TrustedTypePolicy;

/**
 * Web Workers require a sanitized script URL via the Trusted Types API
 * in order to protect against things like XSS.
 */
function getTrustedWorkerURL(url: string): TrustedScriptURL | URL | string {
  if (typeof window === 'undefined') return url;

  const tt = window.trustedTypes;
  if (!tt) return url;

  if (!graphWorkerPolicy) {
    try {
      graphWorkerPolicy = tt.createPolicy('chronicle-graph-worker', {
        createScriptURL: (u: string) => u,
      }) as TrustedTypePolicy;
    } catch {
      return url;
    }
  }

  if (graphWorkerPolicy) {
    return graphWorkerPolicy.createScriptURL(url.toString());
  }

  return url;
}

function Graph() {
  const {
    graph,
    workflowType,
    setWorkflowType,
    selectedNodeId,
    setSelectedNodeId,
  } = useContext(ChronicleContext);
  const [nodes, setNodes, onNodesChange] = useNodesState<ChronicleNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const { fitView } = useReactFlow();
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');
  const [showAssignmentEdges, setShowAssignmentEdges] = useState(false);
  const [autoFitSelection, setAutoFitSelection] = useState(true);
  const [contextMenuState, setContextMenuState] = useState<
    ContextMenuState | undefined
  >(undefined);
  const [errorMessage, setErrorMessage] = useState<string | undefined>(
    undefined,
  );
  const [isLoading, setIsLoading] = useState(false);
  const [baseLayout, setBaseLayout] = useState<{
    nodes: ChronicleNode[];
    edges: Edge[];
  }>({ nodes: [], edges: [] });

  // Track if we have applied the defaults for the current workflow.
  // Used to prevent overriding the graph (eg. collapsed/expanded nodes) after
  // interacted by the user.
  const hasInitializedDefaults = useRef(false);

  // Ref for nodes that are intended to be focused. This is used to centralize
  // the focusing of nodes on a single effect to avoid race conditions between
  // multiple effects trying to focus different sets of nodes.
  const pendingFocusNodes = useRef<string[] | undefined>(undefined);

  // Ref to store a pending fitView request
  const pendingFitViewOptions = useRef<FitViewOptions | undefined>(undefined);

  const {
    groupModes,
    groupData,
    actions: groupActions,
  } = useCollapsibleGroups(graph);

  // While we're still using canned fake data, we need to re-initialize defaults
  // when changing workflow type.
  useEffect(() => {
    // Only reset if we have already initialized once to avoid reset on initial page load.
    if (hasInitializedDefaults.current) {
      hasInitializedDefaults.current = false;
      setSelectedNodeId(undefined);
    }
  }, [workflowType, setSelectedNodeId]);

  // The graph layout algorithm is a performance bottleneck so use the Web Worker API
  // to perform this work in a background thread to prevent freezing the UI.
  useEffect(() => {
    if (!graph) {
      return;
    }

    setIsLoading(true);
    const worker = new Worker(getTrustedWorkerURL(graphWorkerUrl) as URL, {
      type: 'module',
    });

    worker.postMessage({
      graph,
      options: {
        showAssignmentEdges,
        groupModes,
      },
    });

    worker.onmessage = (e) => {
      if (e.data.error) {
        setErrorMessage(`Failed to render graph: ${e.data.error}`);
      } else {
        const { nodes: calculatedNodes, edges: calculatedEdges } = e.data;
        setBaseLayout({ nodes: calculatedNodes, edges: calculatedEdges });
      }
      setIsLoading(false);
      worker.terminate();
    };

    // useEffect cleanup function
    return () => {
      worker.terminate();
    };
  }, [graph, showAssignmentEdges, groupModes]);

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
    let nextNodes = baseLayout.nodes;
    let nextEdges = baseLayout.edges;
    let nodesToFit: string[] = [];

    // Check if there is a pending focus request from an expand/collapse action.
    if (pendingFocusNodes.current && !selectedNodeId) {
      const targetsExist = pendingFocusNodes.current.every((id) =>
        baseLayout.nodes.some((n) => n.id === id),
      );
      if (targetsExist) {
        nodesToFit = pendingFocusNodes.current;
        pendingFocusNodes.current = undefined;
      }
    }

    if (selectedNodeId) {
      const relatedNodeIds = new Set<string>([selectedNodeId]);
      const relatedEdgeIds = new Set<string>();

      // Find immediate neighbors and connecting edges
      baseLayout.edges.forEach((edge) => {
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

    // Queue a fitView request.
    if (nodesToFit.length > 0) {
      pendingFitViewOptions.current = {
        nodes: nodesToFit.map((id) => ({ id })),
        duration: 500,
      };
    } else {
      pendingFitViewOptions.current = {
        duration: 500,
      };
    }
  }, [
    baseLayout,
    selectedNodeId,
    debouncedSearchQuery,
    setNodes,
    setEdges,
    autoFitSelection,
  ]);

  // Effect to process pending fitView requests.
  // It attempts to call fitView() repeatedly until it returns success.
  useEffect(() => {
    if (pendingFitViewOptions.current) {
      const options = pendingFitViewOptions.current;
      let attempts = 0;
      const maxAttempts = 50;

      const tryFitView = async () => {
        // fitView returns true if it successfully calculated the view
        const success = await fitView(options);
        if (success) {
          pendingFitViewOptions.current = undefined;
        } else if (attempts < maxAttempts) {
          attempts++;
          window.requestAnimationFrame(tryFitView);
        } else {
          pendingFitViewOptions.current = undefined;
        }
      };

      window.requestAnimationFrame(tryFitView);
    }
  }, [nodes, fitView]);

  // Use useCallback even with no dependencies to prevent React creating a new
  // function reference on every render.
  // https://reactflow.dev/learn/advanced-use/performance#memoize-functions
  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      setSelectedNodeId(node.id);
      setContextMenuState(undefined);
    },
    [setSelectedNodeId],
  );

  const onNodeContextMenu = useCallback(
    (event: React.MouseEvent, node: ChronicleNode) => {
      event.preventDefault();

      const isSelfCollapsed = !!node.data.isCollapsed;
      const selfGroupId = node.data.groupId;

      // In order to support collapsing children of a node that itself is collapsed,
      // we must get one of the check IDs (first one works) from the current group.
      let representativeId = node.id;
      if (isSelfCollapsed && selfGroupId !== undefined) {
        const members = groupData.groupIdToChecks.get(selfGroupId);
        if (members && members.length > 0) {
          representativeId = members[0].identifier!.id!;
        }
      }

      const childGroups = new Set<number>();
      const groups = groupData.parentToGroupIds.get(representativeId);
      if (groups) {
        groups.forEach((g) => childGroups.add(g));
      }

      // Only show menu if this node itself is collapsed or it has collapsible children.
      if (isSelfCollapsed || childGroups.size > 0) {
        setContextMenuState({
          mouseX: event.clientX - 2,
          mouseY: event.clientY - 4,
          node,
          selfGroupId,
          childGroupIds: Array.from(childGroups),
        });
      } else {
        setContextMenuState(undefined);
      }
    },
    [groupData],
  );

  const onPaneClick = useCallback(() => {
    setSelectedNodeId(undefined);
    setContextMenuState(undefined);
  }, [setSelectedNodeId]);

  const onInspectorClose = useCallback(() => {
    setSelectedNodeId(undefined);
  }, [setSelectedNodeId]);

  const handleContextMenuClose = useCallback(() => {
    setContextMenuState(undefined);
  }, []);

  const handleGroupModeChange = useCallback(
    (groupIds: number[], mode: GroupMode, anchorNodeId?: string) => {
      groupActions.updateModes(groupIds, mode);

      if (mode === GroupMode.EXPANDED) {
        // When expanding, focus on all the newly expanded nodes.
        const nodesToFocus: string[] = [];
        groupIds.forEach((id) => {
          const checks = groupData.groupIdToChecks.get(id);
          if (checks) {
            checks.forEach((c) => {
              if (c.identifier?.id) {
                nodesToFocus.push(c.identifier.id);
              }
            });
          }
        });

        if (nodesToFocus.length > 0) {
          pendingFocusNodes.current = nodesToFocus;
        }
        setSelectedNodeId(undefined);
      } else if (anchorNodeId) {
        // On collapse, focus back to the anchor node (where the action was taken).
        setSelectedNodeId(anchorNodeId);
      }
    },
    [groupActions, groupData, setSelectedNodeId],
  );

  const selectedNode = useMemo(() => {
    if (!nodes || nodes.length === 0) return;
    const n = nodes.find((n) => n.id === selectedNodeId);
    if (!n && !!selectedNodeId) {
      setErrorMessage(`Node "${selectedNodeId}" not found.`);
    }
    return n;
  }, [nodes, selectedNodeId]);

  return (
    <PanelGroup
      direction="horizontal"
      style={{ height: '100%', width: '100%' }}
    >
      <Panel minSize={50}>
        <div style={{ position: 'relative', width: '100%', height: '100%' }}>
          {isLoading && (
            <div
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: '100%',
                backgroundColor: 'rgba(255, 255, 255, 0.7)',
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                zIndex: 1000,
              }}
            >
              <CircularProgress />
            </div>
          )}
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
            onlyRenderVisibleElements={true}
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
                    <MenuItem value={WorkflowType.ANDROID_GIGANTIC_POSTSUBMIT}>
                      Android Gigantic Postsubmit
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
                    <Typography variant="body2">
                      Show Assignment Edges
                    </Typography>
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
                    <Typography variant="body2">
                      Fit View on Selection
                    </Typography>
                  }
                />
                <Button
                  onClick={groupActions.collapseAllSuccessful}
                  sx={{ mt: 1 }}
                  size="small"
                >
                  Collapse Successful
                </Button>
                <Button onClick={groupActions.collapseAll} size="small">
                  Collapse All
                </Button>
                <Button onClick={groupActions.expandAll} size="small">
                  Expand All
                </Button>
              </Paper>
            </ReactFlowPanel>
          </ReactFlow>
        </div>
        <ContextMenu
          contextMenuState={contextMenuState}
          onClose={handleContextMenuClose}
          onSetGroupMode={handleGroupModeChange}
        />
      </Panel>
      {selectedNodeId &&
        !selectedNodeId.startsWith('collapsed-group') &&
        selectedNode && (
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
                <Box
                  sx={{ width: '2px', height: '24px', bgcolor: 'divider' }}
                />
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
      <Snackbar
        open={!!errorMessage}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert
          severity="error"
          onClose={() => {
            setErrorMessage(undefined);
          }}
        >
          {errorMessage}
        </Alert>
      </Snackbar>
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
