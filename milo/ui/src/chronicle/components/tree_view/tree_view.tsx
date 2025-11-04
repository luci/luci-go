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

import AutorenewIcon from '@mui/icons-material/Autorenew';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ErrorIcon from '@mui/icons-material/Error';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import MoreHorizIcon from '@mui/icons-material/MoreHoriz';
import SearchIcon from '@mui/icons-material/Search';
import { Box } from '@mui/material';
import { styled } from '@mui/material/styles';
import { memo, useCallback, useMemo, useRef, KeyboardEvent } from 'react';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { StageView } from '@/proto/turboci/graph/orchestrator/v1/stage_view.pb';

import { FakeGraphGenerator } from '../../fake_turboci_graph';
import { InspectorPanel } from '../inspector_panel/inspector_panel';

import {
  buildVisualGraph,
  FlatTreeItem,
  GraphNode,
  NodeStatus,
  subtreeSize,
} from './build_tree';
import { useTree } from './use_tree';

// Fake graph. Will need to be replaced with a real one eventually.
const graphGenerator = new FakeGraphGenerator({
  workPlanIdStr: 'test-plan',
  numBuilds: 30,
  avgTestsPerBuild: 3,
  numSourceChanges: 3,
});
const turboCiGraph = graphGenerator.generate();

const Icons = {
  // Grey for UI elements.
  ChevronRight: () => <ChevronRightIcon sx={{ color: '#999' }} />,
  ChevronDown: () => <ExpandMoreIcon sx={{ color: '#999' }} />,
  // Green for success.
  Success: () => <CheckCircleIcon sx={{ color: '#28a745' }} />,
  // Red for failure.
  Failure: () => <ErrorIcon sx={{ color: '#dc3545' }} />,
  // Blue for running.
  Running: () => <AutorenewIcon sx={{ color: '#007bff' }} />,
  // Light grey for pending.
  Pending: () => <FiberManualRecordIcon sx={{ color: '#ccc' }} />,
  // Light grey for unknown.
  Unknown: () => <HelpOutlineIcon sx={{ color: '#ccc' }} />,
  // Grey for UI elements.
  More: () => <MoreHorizIcon sx={{ color: '#999' }} />,
  // Grey for UI elements.
  Search: () => <SearchIcon sx={{ color: '#999' }} />,
};

const StatusIcon = ({ status }: { status: NodeStatus }) => {
  switch (status) {
    // Note: FINAL does not mean passed
    // TODO(phobbs): We need a way to read the status of the check / stage and
    // convert to pass/fail/other.
    case 'FINAL':
      return <Icons.Success />;
    case 'ATTEMPTING':
      return <Icons.Running />;
    case 'AWAITING_GROUP':
      return <Icons.Pending />;
    case 'PLANNED':
      return <Icons.Pending />;
    default:
      return <Icons.Unknown />;
  }
};

const StyledTreeRowRoot = styled(Box, {
  shouldForwardProp: (prop) =>
    prop !== 'depth' &&
    prop !== 'isMatch' &&
    prop !== 'isSelected' &&
    prop !== 'isRepeated',
})<{
  depth: number;
  isMatch: boolean;
  isSelected: boolean;
  isRepeated: boolean;
}>(({ depth, isMatch, isSelected, isRepeated }) => ({
  paddingLeft: `${depth * 24}px`,
  display: 'flex',
  alignItems: 'center',
  paddingTop: '4px',
  paddingBottom: '4px',
  cursor: 'pointer',
  fontFamily: 'sans-serif',
  fontSize: '14px',
  borderBottom: '1px solid #eee',
  outline: 'none',
  whiteSpace: 'nowrap',
  backgroundColor: isSelected
    ? '#e6f7ff'
    : isMatch
      ? '#fffce0'
      : isRepeated
        ? '#f9f9f9'
        : 'transparent',
  boxShadow: isSelected ? 'inset 3px 0 0 #1890ff' : 'none',
  opacity: isRepeated ? 0.8 : 1,
  '&:hover': {
    backgroundColor: isSelected ? undefined : 'action.hover',
  },
}));

const StyledRepeatedTreeRowRoot = styled(Box, {
  shouldForwardProp: (prop) => prop !== 'depth' && prop !== 'isSelected',
})<{ depth: number; isSelected: boolean }>(({ depth, isSelected }) => ({
  paddingLeft: `${depth * 24}px`,
  display: 'flex',
  alignItems: 'center',
  paddingTop: '4px',
  paddingBottom: '4px',
  cursor: 'pointer',
  fontFamily: 'sans-serif',
  fontSize: '14px',
  borderBottom: '1px solid #eee',
  outline: 'none',
  whiteSpace: 'nowrap',
  color: '#888',
  fontStyle: 'italic',
  backgroundColor: isSelected ? '#e6f7ff' : '#fcfcfc',
}));

// Presentation component for a single row
const TreeRow = memo(
  ({
    item,
    node,
    index,
    isSelected,
    isExpanded,
    onClick,
    onToggle,
    subtreeSizeForRepeat,
    treeContainerRef,
  }: {
    item: FlatTreeItem;
    node: GraphNode;
    index: number;
    isSelected: boolean;
    isExpanded: boolean;
    onClick: () => void;
    onToggle: () => void;
    subtreeSizeForRepeat?: number;
    treeContainerRef: React.RefObject<HTMLDivElement | null>;
  }) => {
    if (item.isRepeated && !isExpanded) {
      return (
        <StyledRepeatedTreeRowRoot
          id={`tree-row-${index}`}
          depth={item.depth}
          isSelected={isSelected}
          onClick={() => {
            onToggle();
            treeContainerRef.current?.focus({ preventScroll: true });
          }}
          role="button"
          tabIndex={0}
        >
          <Box
            sx={{
              width: '20px',
              marginRight: '4px',
              display: 'flex',
              justifyContent: 'center',
              opacity: 0.5,
            }}
          >
            <Icons.More />
          </Box>
          <Box sx={{ width: '32px' }} />
          <div>
            ... {subtreeSizeForRepeat} repeated{' '}
            {subtreeSizeForRepeat === 1 ? 'node' : 'nodes'}
          </div>
        </StyledRepeatedTreeRowRoot>
      );
    }

    return (
      <StyledTreeRowRoot
        id={`tree-row-${index}`}
        depth={item.depth}
        isMatch={item.isMatch}
        isSelected={isSelected}
        isRepeated={item.isRepeated}
        // After any click action, we immediately move focus back to the
        // main tree container. This is crucial for ensuring keyboard
        // navigation shortcuts (which are handled by the container)
        // continue to work reliably. See the "Focus Management Strategy"
        // comment above the `Tree` component for a more detailed explanation.
        onClick={() => {
          onClick();
          treeContainerRef.current?.focus({ preventScroll: true });
        }}
        role="treeitem"
        // Screen readers know which virtual item is selected even though focus
        // is on the container.
        aria-expanded={item.hasChildren ? isExpanded : undefined}
        aria-level={item.depth + 1}
        aria-selected={isSelected}
        tabIndex={0}
      >
        <Box
          onClick={(e) => {
            e.stopPropagation();
            onToggle();
            treeContainerRef.current?.focus({ preventScroll: true });
          }}
          role="button"
          tabIndex={0}
          sx={{
            width: '20px',
            marginRight: '4px',
            display: 'flex',
            justifyContent: 'center',
            cursor: item.hasChildren ? 'pointer' : 'default',
          }}
        >
          {item.hasChildren && !item.isCycle ? (
            isExpanded ? (
              <Icons.ChevronDown />
            ) : (
              <Icons.ChevronRight />
            )
          ) : (
            <FiberManualRecordIcon sx={{ color: '#eee' }} />
          )}
        </Box>
        <Box
          sx={{
            width: '24px',
            marginRight: '8px',
            display: 'flex',
            justifyContent: 'center',
          }}
        >
          <StatusIcon status={node.status} />
        </Box>
        <Box sx={{ flex: 1, display: 'flex', alignItems: 'baseline' }}>
          <Box
            component="span"
            sx={{
              fontWeight: item.isMatch
                ? 700
                : node.type === 'CHECK'
                  ? 600
                  : 400,
              marginRight: '8px',
            }}
          >
            {node.label}
          </Box>
          {node.type === 'STAGE' && (
            <Box component="span" sx={{ fontSize: '12px', color: '#666' }}>
              [Stage]
            </Box>
          )}
          {item.isCycle && (
            <Box
              component="span"
              sx={{ fontSize: '11px', color: '#999', marginLeft: '8px' }}
            >
              (cycle)
            </Box>
          )}
        </Box>
      </StyledTreeRowRoot>
    );
  },
);
TreeRow.displayName = 'TreeRow';

const StyledInput = styled('input', {
  shouldForwardProp: (prop) => prop !== 'hasSearchQuery',
})<{ hasSearchQuery: boolean }>(({ hasSearchQuery }) => ({
  width: '80%',
  padding: '8px 8px 8px 34px',
  border: '1px solid #ddd',
  borderRadius: '4px',
  outline: 'none',
  boxShadow: hasSearchQuery ? '0 0 0 2px rgba(24,144,255,0.2)' : 'none',
  borderColor: hasSearchQuery ? '#1890ff' : '#ddd',
  transition: 'all 0.2s',
}));

// Focus Management Strategy:
// The main tree container (`treeContainerRef`) is the single source of truth for
// keyboard navigation. It has `tabIndex={0}` and an `onKeyDown` handler that
// implements all the navigation logic (j, k, h, l, etc.).
//
// Individual tree rows (`TreeRow`) are also interactive (`role="button"`) and
// are therefore required by accessibility standards (and lint rules) to be
// focusable (`tabIndex={0}`).
//
// This creates a potential problem: if a user clicks on a row, that row
// receives focus. When the row has focus, the main container's `onKeyDown`
// handler will not fire, and the navigation shortcuts will stop working.
//
// To solve this, whenever a row or any interactive element within it is
// clicked, we programmatically move focus *back* to the main tree container
// (`treeContainerRef.current?.focus()`). This ensures that the main `onKeyDown`
// handler is always active, while still allowing the rows to be individually
// focusable for accessibility purposes.
function Tree() {
  useDeclareTabId('tree');

  const graph = useMemo(() => {
    const stages = Object.values(turboCiGraph.stages) as StageView[];
    const checks = Object.values(turboCiGraph.checks) as CheckView[];
    return buildVisualGraph(stages, checks);
  }, []);

  const {
    visibleItems,
    selectedKey,
    setSelectedKey,
    expandedIds,
    toggleKey,
    handleKeyDown,
    searchQuery,
    setSearchQuery,
  } = useTree({ graph });

  const searchInputRef = useRef<HTMLInputElement>(null);
  const treeContainerRef = useRef<HTMLDivElement>(null);

  const selectedItem = useMemo(() => {
    return visibleItems.find((item) => item.key === selectedKey);
  }, [visibleItems, selectedKey]);

  const selectedNode = useMemo(() => {
    return selectedItem ? graph.nodes[selectedItem.id] : undefined;
  }, [graph, selectedItem]);

  const onInspectorClose = useCallback(() => {
    setSelectedKey(null);
  }, [setSelectedKey]);

  const onTreeKeyDown = (e: KeyboardEvent) => {
    // '/' to focus search
    if (e.key === '/') {
      e.preventDefault();
      searchInputRef.current?.focus();
      return;
    }
    // ESC to clear active search and keep focus on tree
    if (e.key === 'Escape' && searchQuery) {
      e.preventDefault();
      setSearchQuery('');
      return;
    }
    handleKeyDown(e);
  };

  const StyledKbd = styled('kbd')({
    backgroundColor: '#eee',
    borderRadius: '3px',
    padding: '1px 3px',
    fontFamily: 'monospace',
    fontSize: '10px',
    border: '1px solid #ccc',
  });
  return (
    <Box sx={{ display: 'flex', flexDirection: 'row', minHeight: '100%' }}>
      <Box
        sx={{
          flex: 1,
          minWidth: 0,
          maxWidth: '100%',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Box
          sx={{
            padding: '15px',
            borderBottom: '1px solid #eee',
          }}
        >
          <Box sx={{ marginBottom: '15px', position: 'relative' }}>
            <Box
              sx={{
                position: 'absolute',
                left: '10px',
                top: '50%',
                transform: 'translateY(-50%)',
                pointerEvents: 'none',
              }}
            >
              <Icons.Search />
            </Box>
            <StyledInput
              ref={searchInputRef}
              type="text"
              placeholder="Filter nodes... (/ to focus)"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => {
                // Enter freezes filter (by moving focus), ESC clears it.
                if (e.key === 'Enter' || e.key === 'Escape') {
                  e.preventDefault();
                  if (e.key === 'Escape') {
                    setSearchQuery('');
                  }
                  treeContainerRef.current?.focus({ preventScroll: true });
                }
              }}
              hasSearchQuery={!!searchQuery}
            />
          </Box>

          <Box sx={{ fontSize: '12px', color: '#666', lineHeight: '1.5' }}>
            <Box>
              <StyledKbd>/</StyledKbd> filter, <StyledKbd>esc</StyledKbd> clear
              filter, <StyledKbd>enter</StyledKbd> freeze filter
            </Box>
            <Box>
              <StyledKbd>j</StyledKbd> / <StyledKbd>k</StyledKbd> move,{' '}
              <StyledKbd>h</StyledKbd>/<StyledKbd>l</StyledKbd> collapse/expand,{' '}
              <StyledKbd>o</StyledKbd> toggle
            </Box>
            <Box>
              <StyledKbd>J</StyledKbd> /<StyledKbd>K</StyledKbd> next/prev
              sibling, <StyledKbd>O</StyledKbd> recursive toggle,{' '}
              <StyledKbd>L</StyledKbd> recursive expand,{' '}
              <StyledKbd>H</StyledKbd> collapse parent
            </Box>
          </Box>
        </Box>

        <Box
          ref={treeContainerRef}
          tabIndex={0}
          role="tree"
          onKeyDown={onTreeKeyDown}
          sx={{
            outline: 'none',
          }}
        >
          {visibleItems.length === 0 ? (
            <Box sx={{ padding: '20px', color: '#666', textAlign: 'center' }}>
              {searchQuery
                ? 'No nodes match your filter.'
                : 'No graph data available.'}
            </Box>
          ) : (
            visibleItems.map((item, index) => {
              const node = graph.nodes[item.id];
              const isCollapsedRepeat =
                item.isRepeated && !expandedIds.has(item.id);
              const size = isCollapsedRepeat
                ? subtreeSize(graph, item.id)
                : undefined;

              return (
                <TreeRow
                  key={item.key}
                  index={index}
                  item={item}
                  node={node}
                  isSelected={selectedKey === item.key}
                  isExpanded={expandedIds.has(item.id)}
                  subtreeSizeForRepeat={size}
                  treeContainerRef={treeContainerRef}
                  onClick={() => setSelectedKey(item.key)}
                  onToggle={() => {
                    setSelectedKey(item.key);
                    toggleKey(item.key);
                  }}
                />
              );
            })
          )}
        </Box>
      </Box>

      {/* Right Column: Floating Inspector Panel */}
      {selectedNode && (
        <Box
          component="aside"
          aria-label="Inspector Panel"
          sx={{
            width: '450px', // Fixed width for the details pane
            flexShrink: 0,
            borderLeft: '1px solid #eee',
            position: 'sticky', // Inspector panel floats on the right.
            top: 0, // Adjust this value if we need a fixed header (e.g. top: '64px')
            height: '100vh', // Occupy full viewport height
            zIndex: 1,
          }}
        >
          <InspectorPanel
            nodeId={selectedNode.id}
            viewData={selectedNode.raw}
            onClose={onInspectorClose}
          />
        </Box>
      )}
    </Box>
  );
}

export { Tree as Component };
