// Copyright 2026 The LUCI Authors.
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

import { Alert, Box, CircularProgress, Typography } from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { TestInvestigationViewHandle } from '../test_aggregation_viewer';

import { TriageViewProvider, useTriageViewContext } from './context';
import { TriageRow } from './triage_row';

export interface TriageViewUIProps {
  initialExpandedIds?: string[];
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict;
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
    isError,
    error,
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
        (node) => node.id === scrollRequest.id,
      );

      if (index !== -1) {
        lastScrolledRef.current = scrollRequest;
        setTimeout(() => {
          rowVirtualizer.scrollToIndex(index, { align: 'center' });
        }, 50);
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
        ) : isError ? (
          <Box p={2}>
            <Alert severity="error">
              {(error as Error)?.message ||
                'An error occurred while fetching test results.'}
            </Alert>
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
