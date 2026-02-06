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

import { Box, CircularProgress, Typography } from '@mui/material';

import { TestAggregationProvider, useTestAggregationContext } from './context';
import { TestAggregationToolbar } from './test_aggregation_toolbar';
import { TestAggregationVirtualTree } from './test_aggregation_virtual_tree';

export interface TestAggregationViewerProps {
  initialExpandedIds?: string[];
}

export function TestAggregationViewer(props: TestAggregationViewerProps) {
  return (
    <TestAggregationProvider {...props}>
      <TestAggregationDataDisplay />
    </TestAggregationProvider>
  );
}

function TestAggregationDataDisplay() {
  const { flattenedItems, isLoading } = useTestAggregationContext();
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <TestAggregationToolbar />
      <Box
        sx={{
          flexGrow: 1,
          overflow:
            !isLoading && flattenedItems.length === 0 ? 'auto' : 'hidden',
        }}
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
          <TestAggregationVirtualTree />
        )}
      </Box>
    </Box>
  );
}
