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

import MenuIcon from '@mui/icons-material/Menu';
import { Drawer, IconButton, Paper, Tooltip, useTheme } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import React, { useCallback, useMemo, useState, useEffect } from 'react';
import { useNavigate } from 'react-router';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import { useRawInvocationId } from '@/test_investigation/context/context';

import {
  buildHierarchyTree,
  buildFailureReasonTree,
  HierarchyBuildResult,
} from '../../utils/drawer_tree_utils';

import { DrawerContent } from './drawer_content';

const DRAWER_WIDTH_OPEN = 360;

export function TestNavigationDrawer() {
  const navigate = useNavigate();

  const theme = useTheme();
  const [isOpen, setIsOpen] = useState(false);
  const [selectedTab, setSelectedTab] = useState(0);
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());

  const resultDbClient = useResultDbClient();

  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const rawInvocationId = useRawInvocationId();

  const { data: testVariantsResponse, isPending: isLoadingTestVariants } =
    useQuery<QueryTestVariantsResponse | null, Error, readonly TestVariant[]>({
      ...resultDbClient.QueryTestVariants.query(
        QueryTestVariantsRequest.fromPartial({
          invocations: [invocation.name],
          resultLimit: 100,
          pageSize: 1000,
          readMask: [
            'test_id',
            'variant_hash',
            'variant.def',
            'test_metadata.name',
            'results.*.result.status_v2',
            'results.*.result.failure_reason.primary_error_message',
            'status',
            'results.*.result.expected',
          ],
          orderBy: 'status_v2_effective',
        }),
      ),
      enabled: !!invocation.name,
      staleTime: 5 * 60 * 1000,
      select: (data) => data?.testVariants || [],
    });

  const testVariants: readonly TestVariant[] = useMemo(
    () => testVariantsResponse || [],
    [testVariantsResponse],
  );

  const { tree: hierarchyTreeData, idsToExpand: hierarchyIdsToExpand } =
    useMemo((): HierarchyBuildResult => {
      return buildHierarchyTree(
        testVariants,
        testVariant.testId,
        testVariant.variantHash,
      );
    }, [testVariants, testVariant.testId, testVariant.variantHash]);

  const failureReasonTreeData = useMemo(
    () => buildFailureReasonTree(testVariants),
    [testVariants],
  );

  // Effect to update expanded nodes when the current item or its expansion path changes
  useEffect(() => {
    if (hierarchyIdsToExpand && hierarchyIdsToExpand.length > 0) {
      setExpandedNodes(new Set(hierarchyIdsToExpand));
    }
  }, [hierarchyIdsToExpand]);

  const handleToggleDrawer = () => setIsOpen(!isOpen);
  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) =>
    setSelectedTab(newValue);

  const toggleNodeExpansion = useCallback((nodeId: string) => {
    setExpandedNodes((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(nodeId)) newSet.delete(nodeId);
      else newSet.add(nodeId);
      return newSet;
    });
  }, []);

  const handleDrawerTestSelection = (
    selectedTestId: string,
    selectedVariantHash: string,
  ) => {
    if (rawInvocationId && selectedTestId && selectedVariantHash) {
      navigate(
        `/ui/test-investigate/invocations/${rawInvocationId}/tests/${encodeURIComponent(selectedTestId)}/variants/${selectedVariantHash}`,
      );
    }
  };

  return (
    <>
      <Paper
        elevation={isOpen ? 0 : 4}
        sx={{
          position: 'fixed',
          top: '50%',
          left: isOpen ? DRAWER_WIDTH_OPEN : 0,
          transform: 'translateY(-50%)',
          zIndex: theme.zIndex.drawer + (isOpen ? 1 : 2),
          borderTopLeftRadius: 0,
          borderBottomLeftRadius: 0,
          borderTopRightRadius: '8px',
          borderBottomRightRadius: '8px',
          transition: theme.transitions.create('left', {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.enteringScreen,
          }),
          bgcolor: 'background.paper',
        }}
      >
        <Tooltip
          title={isOpen ? 'Close navigation' : 'Open navigation'}
          placement="right"
        >
          <IconButton
            onClick={handleToggleDrawer}
            size="medium"
            sx={{ p: 0.75 }}
          >
            <MenuIcon />
          </IconButton>
        </Tooltip>
      </Paper>

      <Drawer
        variant="temporary"
        anchor="left"
        open={isOpen}
        onClose={handleToggleDrawer}
        sx={{
          width: DRAWER_WIDTH_OPEN,
          flexShrink: 0,
          '& .MuiDrawer-paper': {
            width: DRAWER_WIDTH_OPEN,
            boxSizing: 'border-box',
            height: '100%',
            boxShadow: theme.shadows[5],
            overflow: 'hidden',
          },
        }}
        ModalProps={{ keepMounted: true }}
      >
        <DrawerContent
          selectedTab={selectedTab}
          onTabChange={handleTabChange}
          isLoadingTestVariants={isLoadingTestVariants}
          hierarchyTreeData={hierarchyTreeData}
          failureReasonTreeData={failureReasonTreeData}
          expandedNodes={expandedNodes}
          toggleNodeExpansion={toggleNodeExpansion}
          currentTestId={testVariant.testId}
          currentVariantHash={testVariant.variantHash}
          onSelectTestVariant={handleDrawerTestSelection}
          closeDrawer={handleToggleDrawer}
        />
      </Drawer>
    </>
  );
}
