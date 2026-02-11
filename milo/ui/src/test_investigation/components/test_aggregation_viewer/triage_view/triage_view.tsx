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
  ExpandLess,
  ExpandMore,
  KeyboardArrowDown,
  KeyboardArrowRight,
} from '@mui/icons-material';
import {
  Box,
  CircularProgress,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';

import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { TestInvestigationViewHandle } from '../test_aggregation_viewer';

import { TriageViewNode, useTriageViewContext } from './context/context';
import { TriageViewProvider } from './context/provider';
import { VerdictNode } from './verdict_node';

export interface TriageViewUIProps {
  initialExpandedIds?: string[];
}

export const TriageView = forwardRef<
  TestInvestigationViewHandle,
  TriageViewUIProps
>((props, ref) => {
  return (
    <TriageViewProvider {...props}>
      <TriageViewContent ref={ref} />
    </TriageViewProvider>
  );
});
TriageView.displayName = 'TriageView';

const TriageViewContent = forwardRef<TestInvestigationViewHandle>((_, ref) => {
  const {
    flattenedItems,
    isLoading,
    locateCurrentTest,
    scrollRequest,
    toggleExpansion,
  } = useTriageViewContext();

  useImperativeHandle(ref, () => ({
    locateCurrentTest,
  }));

  const parentRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: flattenedItems.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (index) => {
      const node = flattenedItems[index];
      if (node.type === 'verdict') return 45;
      if (node.type === 'status') return 40;
      return 32; // group
    },
    overscan: 5,
  });

  // Scroll handling
  const lastScrolledRef = useRef<{ id: string; ts: number } | undefined>(
    undefined,
  );
  useEffect(() => {
    if (
      scrollRequest &&
      scrollRequest !== lastScrolledRef.current &&
      flattenedItems.length > 0
    ) {
      const index = flattenedItems.findIndex(
        (node) =>
          (node.type === 'verdict' && node.id === scrollRequest.id) ||
          (node.type === 'status' && node.id === scrollRequest.id) ||
          (node.type === 'group' && node.id === scrollRequest.id),
      );

      if (index !== -1) {
        rowVirtualizer.scrollToIndex(index, { align: 'center' });
        lastScrolledRef.current = scrollRequest;
      }
    }
  }, [scrollRequest, flattenedItems, rowVirtualizer]);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Toolbar is hosted in parent */}
      <Box
        ref={parentRef}
        sx={{ flexGrow: 1, overflow: 'auto', p: 0, position: 'relative' }}
      >
        {isLoading ? (
          <Box display="flex" justifyContent="center" p={2}>
            <CircularProgress />
          </Box>
        ) : flattenedItems.length === 0 ? (
          <Box p={2}>
            <Typography variant="body2" color="text.secondary">
              No failures found.
            </Typography>
          </Box>
        ) : (
          <div
            style={{
              height: `${rowVirtualizer.getTotalSize()}px`,
              width: '100%',
              position: 'relative',
            }}
          >
            {rowVirtualizer.getVirtualItems().map((virtualDesc) => {
              const node = flattenedItems[virtualDesc.index];
              return (
                <div
                  key={`${node.type}-${node.id}`}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    height: `${virtualDesc.size}px`,
                    transform: `translateY(${virtualDesc.start}px)`,
                  }}
                >
                  <TriageRow node={node} toggleExpansion={toggleExpansion} />
                </div>
              );
            })}
          </div>
        )}
      </Box>
    </Box>
  );
});
TriageViewContent.displayName = 'TriageViewContent';

interface TriageRowProps {
  node: TriageViewNode;
  toggleExpansion: (id: string) => void;
}

function TriageRow({ node, toggleExpansion }: TriageRowProps) {
  if (node.type === 'status') {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          height: '100%',
          px: 1,
          bgcolor: 'action.hover',
          borderBottom: '1px solid',
          borderColor: 'divider',
          cursor: 'pointer',
          userSelect: 'none',
        }}
        onClick={() => toggleExpansion(node.id)}
      >
        <IconButton size="small" sx={{ mr: 1, p: 0.5 }}>
          {node.expanded ? (
            <ExpandLess fontSize="small" />
          ) : (
            <ExpandMore fontSize="small" />
          )}
        </IconButton>
        <Typography
          variant="subtitle2"
          sx={{
            fontWeight: 'bold',
            flexGrow: 1,
            ...getStatusStyle(node.id),
          }}
          noWrap
        >
          {node.id} ({node.group.count})
        </Typography>
      </Box>
    );
  }

  if (node.type === 'group') {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          height: '100%',
          pl: 3, // Indent
          pr: 1,
          borderBottom: '1px solid',
          borderColor: 'divider',
          cursor: 'pointer',
          '&:hover': { bgcolor: 'action.hover' },
          bgcolor: 'background.paper',
        }}
        onClick={() => toggleExpansion(node.id)}
      >
        <Box sx={{ mr: 1, display: 'flex', alignItems: 'center' }}>
          {node.expanded ? (
            <KeyboardArrowDown
              fontSize="small"
              sx={{ color: 'text.secondary', fontSize: '1rem' }}
            />
          ) : (
            <KeyboardArrowRight
              fontSize="small"
              sx={{ color: 'text.secondary', fontSize: '1rem' }}
            />
          )}
        </Box>
        <Tooltip title={node.group.reason} placement="right" enterDelay={500}>
          <Typography
            variant="body2"
            sx={{
              fontFamily: 'monospace',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              flexGrow: 1,
              maxWidth: 'calc(100% - 80px)', // Reserve space for count
            }}
          >
            {node.group.reason}
          </Typography>
        </Tooltip>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ ml: 1, flexShrink: 0 }}
        >
          ({node.group.verdicts.length})
        </Typography>
      </Box>
    );
  }

  if (node.type === 'verdict') {
    return (
      <VerdictNode
        verdict={node.verdict}
        currentFailureReason={node.parentGroup}
      />
    );
  }

  return null;
}

function getStatusStyle(status: string) {
  if (status === TestVerdict_Status[TestVerdict_Status.FAILED]) {
    return { color: 'error.main' };
  }
  if (status === TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED]) {
    return { color: 'warning.main' };
  }
  if (status === TestVerdict_Status[TestVerdict_Status.FLAKY]) {
    return { color: 'warning.light' };
  }
  return { color: 'text.primary' };
}
