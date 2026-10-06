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

import { Alert, Box, CircularProgress, Snackbar } from '@mui/material';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  ReactFlowProvider,
  useReactFlow,
  Node,
  FitViewOptions,
  Edge,
  CoordinateExtent,
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

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';

import { computeCriticalPath } from '../utils/critical_path';
import { ChronicleNode, GroupMode } from '../utils/graph_builder';
// ?worker&url is special vite syntax to import a web worker script
// and retrieve the URL to the script.
// eslint-disable-next-line import/default
import graphWorkerUrl from '../utils/graph_worker?worker&url';
import { getBaseNodeId } from '../utils/id';
import { getTrustedWorkerURL } from '../utils/worker_utils';

import { ChronicleContext } from './context';
import { ContextMenu, ContextMenuState } from './context_menu';
import { GraphControlPanel } from './graph_control_panel';
import { useCollapsibleGroups } from './hooks/use_collapsible_groups';
import { useGraphHighlighting } from './hooks/use_graph_highlighting';
import { useNodeSearch } from './hooks/use_node_search';
import { InspectorPanel } from './inspector_panel/inspector_panel';

const SELECTION_FIT_MAX_ZOOM = 0.7;
// Chosen to feel snappy when iterating through search results.
const SELECTION_FIT_DURATION = 200;

function Graph() {
  const {
    graph,
    valueDataMap,
    workflowType,
    selectedNodeId,
    setSelectedNodeId,
  } = useContext(ChronicleContext);
  const [nodes, setNodes, onNodesChange] = useNodesState<ChronicleNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const { fitView } = useReactFlow();
  const [showAssignmentEdges, setShowAssignmentEdges] = useState(false);
  const [showCriticalPath, setShowCriticalPath] = useState(false);
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

  const filteredGraph = useMemo(() => {
    if (!graph || !showCriticalPath) {
      return graph;
    }

    const criticalPathStageIds = computeCriticalPath(graph.stages);
    return {
      ...graph,
      stages: graph.stages.filter(
        (s) => s.identifier?.id && criticalPathStageIds.has(s.identifier.id),
      ),
    };
  }, [graph, showCriticalPath]);

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
  const lastFittedNodeIdRef = useRef<string | undefined>(undefined);

  const {
    groupModes,
    groupData,
    actions: groupActions,
  } = useCollapsibleGroups(filteredGraph);

  // Nodes sorted in left-to-right, top-to-bottom layout order for search match iteration.
  const searchableNodes = useMemo(
    () =>
      [...baseLayout.nodes]
        .sort(
          (a, b) => a.position.x - b.position.x || a.position.y - b.position.y,
        )
        .map((node) => ({
          id: node.id,
          label: node.data.fullLabel,
          view: node.data.view,
        })),
    [baseLayout.nodes],
  );

  const handleSettledMatch = useCallback(
    (targetId: string) => {
      if (autoFitSelection) {
        lastFittedNodeIdRef.current = targetId;
        fitView({
          nodes: [{ id: targetId }],
          duration: SELECTION_FIT_DURATION,
          maxZoom: SELECTION_FIT_MAX_ZOOM,
        });
      }
    },
    [autoFitSelection, fitView],
  );

  const nodeSearch = useNodeSearch({
    nodes: searchableNodes,
    valueDataMap,
    isLoading,
    selectedNodeId,
    setSelectedNodeId,
    onSettledMatch: handleSettledMatch,
  });
  const {
    matchedNodeIds,
    effectiveSettledNodeId,
    cancelPendingMatchNav,
    resetSearchTracking,
  } = nodeSearch;

  // While we're still using canned fake data, we need to re-initialize defaults
  // when changing workflow type.
  useEffect(() => {
    // Only reset if we have already initialized once to avoid reset on initial page load.
    if (hasInitializedDefaults.current) {
      hasInitializedDefaults.current = false;
      resetSearchTracking();
      setSelectedNodeId(undefined);
    }
  }, [workflowType, showCriticalPath, resetSearchTracking, setSelectedNodeId]);

  // The graph layout algorithm is a performance bottleneck so use the Web Worker API
  // to perform this work in a background thread to prevent freezing the UI.
  useEffect(() => {
    if (!filteredGraph) {
      return;
    }

    setIsLoading(true);
    const worker = new Worker(getTrustedWorkerURL(graphWorkerUrl) as URL, {
      type: 'module',
    });

    worker.postMessage({
      graph: filteredGraph,
      valueDataMap,
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
  }, [filteredGraph, valueDataMap, showAssignmentEdges, groupModes]);

  const { baseNodesMap } = useGraphHighlighting({
    baseLayout,
    effectiveSettledNodeId,
    matchedNodeIds,
    setNodes,
    setEdges,
  });

  // Queue a fitView request when layout initializes, groups expand, or selection changes.
  useEffect(() => {
    // While the layout worker is computing, do not run fitView or consume
    // hasInitializedDefaults against stale nodes.
    if (isLoading) {
      return;
    }

    let nodesToFit: string[] = [];

    // Check if there is a pending focus request from an expand/collapse action.
    if (pendingFocusNodes.current && !effectiveSettledNodeId) {
      const targetsExist = pendingFocusNodes.current.every((id) =>
        baseNodesMap.has(id),
      );
      if (targetsExist) {
        nodesToFit = pendingFocusNodes.current;
        pendingFocusNodes.current = undefined;
      }
    }

    if (!autoFitSelection) {
      lastFittedNodeIdRef.current = undefined;
    }

    if (effectiveSettledNodeId) {
      const baseNodeId = getBaseNodeId(effectiveSettledNodeId, {
        includePrefix: false,
      })!;
      // Only autofit on selection if the option is enabled, the node exists in the current layout,
      // and not already fitted immediately.
      if (
        autoFitSelection &&
        baseNodesMap.has(baseNodeId) &&
        lastFittedNodeIdRef.current !== baseNodeId
      ) {
        lastFittedNodeIdRef.current = baseNodeId;
        nodesToFit = [baseNodeId];
      }
    } else {
      lastFittedNodeIdRef.current = undefined;
    }

    if (nodesToFit.length > 0) {
      pendingFitViewOptions.current = {
        nodes: nodesToFit.map((id) => ({ id })),
        duration: SELECTION_FIT_DURATION,
        maxZoom: SELECTION_FIT_MAX_ZOOM,
      };
      if (baseLayout.nodes.length > 0) {
        hasInitializedDefaults.current = true;
      }
    } else if (!hasInitializedDefaults.current && baseLayout.nodes.length > 0) {
      hasInitializedDefaults.current = true;
      pendingFitViewOptions.current = {
        duration: SELECTION_FIT_DURATION,
      };
    }
  }, [
    baseLayout,
    baseNodesMap,
    effectiveSettledNodeId,
    autoFitSelection,
    isLoading,
  ]);

  // Effect to process pending fitView requests.
  // It attempts to call fitView() repeatedly until it returns success.
  useEffect(() => {
    if (pendingFitViewOptions.current) {
      const options = pendingFitViewOptions.current;
      pendingFitViewOptions.current = undefined;
      let attempts = 0;
      const maxAttempts = 50;

      const tryFitView = async () => {
        // fitView returns true if it successfully calculated the view
        const success = await fitView(options);
        if (!success && attempts < maxAttempts) {
          attempts++;
          window.requestAnimationFrame(tryFitView);
        }
      };

      void tryFitView();
    }
  }, [nodes, fitView, autoFitSelection, effectiveSettledNodeId]);

  // Use useCallback even with no dependencies to prevent React creating a new
  // function reference on every render.
  // https://reactflow.dev/learn/advanced-use/performance#memoize-functions
  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      cancelPendingMatchNav();
      const baseSelectedId = selectedNodeId
        ? getBaseNodeId(selectedNodeId, { includePrefix: false })
        : undefined;
      const isAlreadySelected =
        selectedNodeId === node.id || baseSelectedId === node.id;
      setSelectedNodeId(isAlreadySelected ? undefined : node.id);
      setContextMenuState(undefined);
    },
    [cancelPendingMatchNav, selectedNodeId, setSelectedNodeId],
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
    cancelPendingMatchNav();
    setSelectedNodeId(undefined);
    setContextMenuState(undefined);
  }, [cancelPendingMatchNav, setSelectedNodeId]);

  const onInspectorClose = useCallback(() => {
    cancelPendingMatchNav();
    setSelectedNodeId(undefined);
  }, [cancelPendingMatchNav, setSelectedNodeId]);

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
    if (!effectiveSettledNodeId || baseNodesMap.size === 0) return undefined;
    const baseNodeId = getBaseNodeId(effectiveSettledNodeId, {
      includePrefix: false,
    });
    return baseNodeId ? baseNodesMap.get(baseNodeId) : undefined;
  }, [baseNodesMap, effectiveSettledNodeId]);

  useEffect(() => {
    if (
      baseNodesMap.size > 0 &&
      effectiveSettledNodeId &&
      !effectiveSettledNodeId.startsWith('collapsed-group') &&
      !selectedNode
    ) {
      setErrorMessage(`Node "${effectiveSettledNodeId}" not found.`);
    }
  }, [baseNodesMap, effectiveSettledNodeId, selectedNode]);

  const inspectorPanelElement = useMemo(() => {
    if (
      !effectiveSettledNodeId ||
      effectiveSettledNodeId.startsWith('collapsed-group') ||
      !selectedNode
    ) {
      return null;
    }
    return (
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
            nodeId={effectiveSettledNodeId}
            viewData={selectedNode?.data?.view}
            valueDataMap={valueDataMap}
            onClose={onInspectorClose}
          />
        </Panel>
      </>
    );
  }, [effectiveSettledNodeId, selectedNode, valueDataMap, onInspectorClose]);

  const reactFlowStaticChildren = useMemo(
    () => (
      <>
        <Background />
        <Controls />
        <MiniMap
          pannable
          zoomable
          nodeStrokeColor="var(--greyed-out-text-color)"
          nodeStrokeWidth={50}
          maskStrokeColor="var(--active-text-color)"
          maskStrokeWidth={1}
        />
      </>
    ),
    [],
  );

  // Constrain the panning viewport to the bounding box of the graph nodes (with padding).
  // This prevents users from panning out into empty canvas space, automatically centers
  // the graph when zoomed out, and prevents the MiniMap from inflating its bounding box
  // to include empty void.
  const translateExtent = useMemo<CoordinateExtent | undefined>(() => {
    if (nodes.length === 0) return undefined;

    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;

    for (const node of nodes) {
      const x = node.position.x;
      const y = node.position.y;
      const w = node.measured?.width ?? node.initialWidth ?? 240;
      const h = node.measured?.height ?? node.initialHeight ?? 32;

      minX = Math.min(minX, x);
      minY = Math.min(minY, y);
      maxX = Math.max(maxX, x + w);
      maxY = Math.max(maxY, y + h);
    }

    // Comfortable margin so nodes aren't pressed directly against the screen edges
    const PADDING_X = 300;
    const PADDING_Y = 150;

    return [
      [minX - PADDING_X, minY - PADDING_Y],
      [maxX + PADDING_X, maxY + PADDING_Y],
    ];
  }, [nodes]);

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
            translateExtent={translateExtent}
            onlyRenderVisibleElements={true}
          >
            {reactFlowStaticChildren}
            <GraphControlPanel
              search={nodeSearch}
              showAssignmentEdges={showAssignmentEdges}
              onShowAssignmentEdgesChange={setShowAssignmentEdges}
              autoFitSelection={autoFitSelection}
              onAutoFitSelectionChange={setAutoFitSelection}
              showCriticalPath={showCriticalPath}
              onShowCriticalPathChange={setShowCriticalPath}
              onCollapseAllSuccessful={groupActions.collapseAllSuccessful}
              onCollapseAll={groupActions.collapseAll}
              onExpandAll={groupActions.expandAll}
            />
          </ReactFlow>
        </div>
        <ContextMenu
          contextMenuState={contextMenuState}
          onClose={handleContextMenuClose}
          onSetGroupMode={handleGroupModeChange}
        />
      </Panel>
      {inspectorPanelElement}
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
