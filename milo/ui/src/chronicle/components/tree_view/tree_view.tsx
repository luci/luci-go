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
import CancelIcon from '@mui/icons-material/Cancel';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ErrorIcon from '@mui/icons-material/Error';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FiberManualRecordIcon from '@mui/icons-material/FiberManualRecord';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import MoreHorizIcon from '@mui/icons-material/MoreHoriz';
import SearchIcon from '@mui/icons-material/Search';
import { Box, CircularProgress } from '@mui/material';
import { styled } from '@mui/material/styles';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  memo,
  useCallback,
  useMemo,
  useRef,
  KeyboardEvent,
  useContext,
} from 'react';

import { useDeclareTabId } from '@/generic_libs/components/routed_tabs/context';
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';

import { CheckResultStatus, StageResultStatus } from '../../utils/check_utils';
import { ChronicleContext } from '../context';
import { InspectorPanel } from '../inspector_panel/inspector_panel';

import {
  buildVisualGraph,
  FlatTreeItem,
  GraphNode,
  subtreeSize,
} from './build_tree';
import { useTree } from './use_tree';

const Icons = {
  // Grey for UI elements.
  ChevronRight: () => <ChevronRightIcon sx={{ color: '#999' }} />,
  ChevronDown: () => <ExpandMoreIcon sx={{ color: '#999' }} />,
  // Green for success.
  Success: () => <CheckCircleIcon sx={{ color: 'var(--success-color)' }} />,
  // Red for failure.
  Failure: () => <ErrorIcon sx={{ color: 'var(--failure-color)' }} />,
  // Orange for running.
  Running: () => <AutorenewIcon sx={{ color: 'var(--started-color)' }} />,
  // Blue for canceled.
  Canceled: () => <CancelIcon sx={{ color: 'var(--canceled-color)' }} />,
  // Grey for pending.
  Pending: () => (
    <FiberManualRecordIcon sx={{ color: 'var(--scheduled-color)' }} />
  ),
  // Light grey for unknown.
  Unknown: () => <HelpOutlineIcon sx={{ color: '#ccc' }} />,
  // Grey for UI elements.
  More: () => <MoreHorizIcon sx={{ color: '#999' }} />,
  // Grey for UI elements.
  Search: () => <SearchIcon sx={{ color: '#999' }} />,
};

function getStageStatusIcon(node: GraphNode) {
  switch (node.resultStatus) {
    case StageResultStatus.SUCCESS:
      return <Icons.Success />;
    case StageResultStatus.FAILURE:
      return <Icons.Failure />;
    case StageResultStatus.RUNNING:
      return <Icons.Running />;
    case StageResultStatus.CANCELLED:
      return <Icons.Canceled />;
    case StageResultStatus.PENDING:
      return <Icons.Pending />;
    case StageResultStatus.UNKNOWN:
    default:
      switch (node.status) {
        case 'FINAL':
          return <Icons.Success />;
        case 'ATTEMPTING':
          return <Icons.Running />;
        case 'PLANNED':
        case 'AWAITING_GROUP':
          return <Icons.Pending />;
        default:
          return <Icons.Unknown />;
      }
  }
}

function getCheckStatusIcon(node: GraphNode) {
  switch (node.status) {
    case 'FINAL':
      switch (node.resultStatus) {
        case CheckResultStatus.SUCCESS:
          return <Icons.Success />;
        case CheckResultStatus.FAILURE:
          return <Icons.Failure />;
        default:
          return <Icons.Unknown />;
      }
    case 'ATTEMPTING':
      return <Icons.Running />;
    case 'AWAITING_GROUP':
    case 'PLANNED':
    case 'PLANNING':
    case 'WAITING':
      return <Icons.Pending />;
    default:
      return <Icons.Unknown />;
  }
}

const StatusIcon = ({ node }: { node: GraphNode }) => {
  return node.type === 'STAGE'
    ? getStageStatusIcon(node)
    : getCheckStatusIcon(node);
};

export const TREE_ROW_HEIGHT = 29;

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
  height: `${TREE_ROW_HEIGHT}px`,
  boxSizing: 'border-box',
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
  height: `${TREE_ROW_HEIGHT}px`,
  boxSizing: 'border-box',
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
          <StatusIcon node={node} />
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

  const { graph: turboCiGraph, valueDataMap } = useContext(ChronicleContext);

  const graph = useMemo(() => {
    if (!turboCiGraph) return { nodes: {}, roots: [] };

    const stages = Object.values(turboCiGraph.stages) as Stage[];
    const checks = Object.values(turboCiGraph.checks) as Check[];

    return buildVisualGraph(stages, checks, valueDataMap || new Map());
  }, [turboCiGraph, valueDataMap]);

  const treeContainerRef = useRef<HTMLDivElement>(null);

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

  // Virtualizes the flattened tree list so only nodes currently in (or near) the viewport are mounted into the DOM.
  const rowVirtualizer = useVirtualizer({
    count: visibleItems.length,
    getScrollElement: () => treeContainerRef.current,
    // Estimated row height in pixels (TREE_ROW_HEIGHT = 29px).
    estimateSize: () => TREE_ROW_HEIGHT,
    // Number of additional rows to render above and below the visible viewport.
    // Overscanning prevents blank spaces from flashing when scrolling quickly.
    overscan: 20,
    getItemKey: (index) => visibleItems[index]?.key || index,
  });

  const searchInputRef = useRef<HTMLInputElement>(null);

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
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'row',
        height: 'calc(100vh - 48px)',
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          flex: 1,
          minWidth: 0,
          minHeight: 0,
          maxWidth: '100%',
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          overflow: 'hidden',
        }}
      >
        <Box
          sx={{
            padding: '15px',
            borderBottom: '1px solid #eee',
            flexShrink: 0,
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
            flex: 1,
            minHeight: 0,
            overflow: 'auto',
          }}
        >
          {!turboCiGraph ? (
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
                minHeight: '200px',
              }}
            >
              <CircularProgress />
            </Box>
          ) : visibleItems.length === 0 ? (
            <Box sx={{ padding: '20px', color: '#666', textAlign: 'center' }}>
              {searchQuery
                ? 'No nodes match your filter.'
                : 'No graph data available.'}
            </Box>
          ) : (
            <div
              style={{
                height: `${rowVirtualizer.getTotalSize()}px`,
                width: '100%',
                position: 'relative',
              }}
            >
              {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                const item = visibleItems[virtualRow.index];
                if (!item) return null;
                const node = graph.nodes[item.id];
                const isCollapsedRepeat =
                  item.isRepeated && !expandedIds.has(item.id);
                const size = isCollapsedRepeat
                  ? subtreeSize(graph, item.id)
                  : undefined;

                return (
                  <div
                    key={virtualRow.key}
                    data-index={virtualRow.index}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                  >
                    <TreeRow
                      index={virtualRow.index}
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
                  </div>
                );
              })}
            </div>
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
            height: '100%',
            overflowY: 'auto',
            zIndex: 1,
          }}
        >
          <InspectorPanel
            nodeId={selectedNode.id}
            viewData={selectedNode.raw}
            valueDataMap={valueDataMap}
            onClose={onInspectorClose}
          />
        </Box>
      )}
    </Box>
  );
}

export { Tree as Component };
