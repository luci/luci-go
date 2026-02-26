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
import { forwardRef, useImperativeHandle } from 'react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { TestInvestigationViewHandle } from '../test_aggregation_viewer';

import { useAggregationViewContext } from './context/context';
import { AggregationViewProvider } from './context/provider';
import { TestAggregationVirtualTree } from './test_aggregation_virtual_tree';

export interface AggregationViewProps {
  initialExpandedIds?: string[];
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict;
  autoLocate?: boolean;
}

const AggregationViewContent = forwardRef<
  TestInvestigationViewHandle,
  AggregationViewProps
>((_props, ref) => {
  const { flattenedItems, isLoading, isError, error, locateCurrentTest } =
    useAggregationViewContext();

  useImperativeHandle(ref, () => ({
    locateCurrentTest,
  }));

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Box sx={{ flexGrow: 1, overflow: 'hidden' }}>
        {isLoading ? (
          <Box
            display="flex"
            justifyContent="center"
            p={2}
            data-testid="loading-spinner"
          >
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
          <TestAggregationVirtualTree />
        )}
      </Box>
    </Box>
  );
});
AggregationViewContent.displayName = 'AggregationViewContent';

export const AggregationView = forwardRef<
  TestInvestigationViewHandle,
  AggregationViewProps
>((props, ref) => {
  return (
    <AggregationViewProvider {...props}>
      <AggregationViewContent ref={ref} {...props} />
    </AggregationViewProvider>
  );
});
AggregationView.displayName = 'AggregationView';
