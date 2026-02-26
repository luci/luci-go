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

import { Box, Button, CircularProgress, Typography } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState, useRef } from 'react';
import { useNavigate } from 'react-router';

import { useFeatureFlag } from '@/common/feature_flags/context';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  semanticStatusForTestVariant,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import { logging } from '@/common/tools/logging';
import { generateTestInvestigateUrl } from '@/common/tools/url_utils';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { QueryTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  InvocationCounts,
  TestVariantsTable,
} from '@/test_investigation/components/invocation_page';
import { TestAggregationViewer } from '@/test_investigation/components/test_aggregation_viewer/test_aggregation_viewer';
import { TestNavigationTreeNode } from '@/test_investigation/components/test_navigation_drawer/types';
import {
  useInvocation,
  useIsLegacyInvocation,
  useRawInvocationId,
} from '@/test_investigation/context/context';
import { TEST_AGGREGATION_IN_INVOCATION_FLAG } from '@/test_investigation/pages/features';
import { buildHierarchyTree } from '@/test_investigation/utils/drawer_tree_utils';

/**
 * Recursively filters the tree data based on a set of selected statuses.
 * A parent node is kept if any of its descendants are a match.
 */
function filterTreeByStatus(
  nodes: readonly TestNavigationTreeNode[],
  statuses: Set<string>,
): TestNavigationTreeNode[] {
  if (statuses.size === 0) {
    return [...nodes];
  }

  const filteredNodes: TestNavigationTreeNode[] = [];

  for (const node of nodes) {
    if (node.children?.length) {
      const filteredChildren = filterTreeByStatus(node.children, statuses);
      // Keep the parent if any of its children matched the filter.
      if (filteredChildren.length > 0) {
        filteredNodes.push({ ...node, children: filteredChildren });
      }
    } else if (node.testVariant) {
      if (statuses.has(semanticStatusForTestVariant(node.testVariant))) {
        filteredNodes.push(node);
      }
    }
  }

  return filteredNodes;
}

export function TestTab() {
  useDeclareTabId('tests');
  const invocation = useInvocation();
  const invocationId = useRawInvocationId();
  const isLegacyInvocation = useIsLegacyInvocation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const hasPerformedInitialRedirect = useRef(false);
  const resultDbClient = useResultDbClient();

  const { parsedTestId, parsedVariantDef } = useMemo(() => {
    const parsedTestId = searchParams.get('testId');
    const variantDef: Record<string, string> = {};
    searchParams.getAll('v').forEach((vParam) => {
      const firstColonIndex = vParam.indexOf(':');
      if (firstColonIndex > 0) {
        const key = vParam.substring(0, firstColonIndex);
        const value = vParam.substring(firstColonIndex + 1);
        variantDef[key] = value;
      }
    });

    return {
      parsedTestId: parsedTestId,
      parsedVariantDef: Object.keys(variantDef).length > 0 ? variantDef : null,
    };
  }, [searchParams]);

  const [selectedStatuses, setSelectedStatuses] = useState<
    Set<SemanticStatusType>
  >(() => {
    const statusParam = searchParams.get('status');
    if (statusParam !== null) {
      if (statusParam === '') {
        return new Set();
      }
      return new Set(
        statusParam.split(',').filter((s) => s) as SemanticStatusType[],
      );
    }

    if (parsedTestId || parsedVariantDef) {
      return new Set();
    }
    return new Set(['failed', 'execution_errored']);
  });

  const handleSelectedStatusesChange = (
    newStatuses: Set<SemanticStatusType>,
  ) => {
    setSelectedStatuses(newStatuses);
    setSearchParams(
      (params) => {
        const hasFilter = params.has('testId') || params.getAll('v').length > 0;
        // If filters are present, default is "Show All" (size 0).
        // If filters are absent, default is "Failed + Error".
        const isDefault = hasFilter
          ? newStatuses.size === 0
          : newStatuses.size === 2 &&
            newStatuses.has('failed') &&
            newStatuses.has('execution_errored');

        if (isDefault) {
          params.delete('status');
        } else {
          params.set('status', Array.from(newStatuses).join(','));
        }
        return params;
      },
      { replace: true },
    );
  };

  const aggregationViewAipFilter = useMemo(() => {
    const filterParts: string[] = [];
    if (parsedTestId) {
      filterParts.push(`test_id:${JSON.stringify(parsedTestId)}`);
    }
    if (parsedVariantDef) {
      Object.entries(parsedVariantDef).forEach(([key, value]) => {
        filterParts.push(
          `test_id_structured.module_variant.${key} = ${JSON.stringify(value)}`,
        );
      });
    }
    return filterParts.join(' AND ') || '';
  }, [parsedTestId, parsedVariantDef]);

  const queryRequest = useMemo(() => {
    const filterParts: string[] = [];
    if (parsedTestId) {
      filterParts.push(`test_id:${JSON.stringify(parsedTestId)}`);
    }
    if (parsedVariantDef) {
      Object.entries(parsedVariantDef).forEach(([key, value]) => {
        filterParts.push(`variant.${key} = ${JSON.stringify(value)}`);
      });
    }
    const filter = filterParts.join(' AND ') || undefined;

    const readMask = [
      'test_id',
      'test_id_structured',
      'variant.def',
      'status_v2',
      'results.*.result.failure_reason',
      'results.*.result.skipped_reason',
      'results.*.result.summary_html',
      'results.*.result.status_v2',
      'results.*.result.name',
    ];
    const pageSize = 10000;
    const resultLimit = 10;

    const parentOrInvocation = isLegacyInvocation
      ? `invocations/${invocationId}`
      : `rootInvocations/${invocationId}`;

    if (isLegacyInvocation) {
      return QueryTestVariantsRequest.fromPartial({
        invocations: [parentOrInvocation],
        pageSize,
        resultLimit,
        readMask,
        filter,
      });
    } else {
      return QueryTestVariantsRequest.fromPartial({
        parent: parentOrInvocation,
        pageSize,
        resultLimit,
        readMask,
        filter,
      });
    }
  }, [invocationId, parsedTestId, parsedVariantDef, isLegacyInvocation]);

  const {
    data: testVariantsResponse,
    isFetching: isFetchingTestVariants,
    isSuccess,
    error: testVariantsError,
    isError: isTestVariantsError,
    isPending,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    ...resultDbClient.QueryTestVariants.queryPaged(queryRequest),
    staleTime:
      invocation?.state === Invocation_State.FINALIZED
        ? Infinity
        : 5 * 60 * 1000,
    enabled: !!invocation,
  });

  const isLoadingTestVariantsPage = isPending || isFetchingNextPage;
  const testVariants = useMemo(() => {
    return (
      testVariantsResponse?.pages.flatMap((page) => page.testVariants || []) ||
      []
    );
  }, [testVariantsResponse]);

  const { finalFilteredVariants, finalFilteredTree } = useMemo(() => {
    if (!testVariants) {
      return { finalFilteredVariants: [], finalFilteredTree: [] };
    }

    const linkFilteredVariants = testVariants;
    const { tree } = buildHierarchyTree(linkFilteredVariants);
    const statusFilteredTree = filterTreeByStatus(tree, selectedStatuses);
    return {
      finalFilteredVariants: linkFilteredVariants,
      finalFilteredTree: statusFilteredTree,
    };
  }, [testVariants, selectedStatuses]);

  const isUniqueResult =
    isSuccess &&
    (parsedTestId || parsedVariantDef) &&
    finalFilteredVariants.length === 1;

  useEffect(() => {
    if (isSuccess && !hasPerformedInitialRedirect.current) {
      hasPerformedInitialRedirect.current = true;

      if (!isUniqueResult) {
        return;
      }

      const variantToRedirect = finalFilteredVariants[0];
      const newPath = generateTestInvestigateUrl(
        invocationId,
        variantToRedirect.testIdStructured!,
      );
      navigate(newPath, { replace: true });
    }
  }, [
    isSuccess,
    parsedTestId,
    parsedVariantDef,
    invocationId,
    navigate,
    finalFilteredVariants,
    isUniqueResult,
    isLegacyInvocation,
  ]);

  const showInitialLoader =
    isFetchingTestVariants && !hasPerformedInitialRedirect.current;
  const isRedirecting = isUniqueResult && !hasPerformedInitialRedirect.current;

  const isTestAggregationEnabled = useFeatureFlag(
    TEST_AGGREGATION_IN_INVOCATION_FLAG,
  );

  if (isTestAggregationEnabled) {
    if (!invocation) {
      return (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100vh',
            p: 2,
          }}
        >
          <CircularProgress />
          <Typography sx={{ ml: 2 }}>Loading Invocation...</Typography>
        </Box>
      );
    }
    return (
      <Box
        sx={{
          height: 'calc(100vh - 200px)',
          display: 'flex',
          flexDirection: 'column',
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
        }}
      >
        <TestAggregationViewer
          invocation={invocation}
          autoLocate={true}
          defaultTab={1}
          initialAipFilter={aggregationViewAipFilter}
          initialExpandedIds={[
            'FAILED',
            'EXECUTION_ERRORED',
            'FLAKY',
            'SKIPPED',
            'PRECLUDED',
            'PASSED',
            'EXONERATED',
            'UNEXPECTEDLY_SKIPPED',
          ]}
        />{' '}
      </Box>
    );
  }

  if (showInitialLoader || isRedirecting) {
    return (
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100vh',
          p: 2,
        }}
      >
        <CircularProgress />
        <Typography sx={{ ml: 2 }}>Loading...</Typography>
      </Box>
    );
  }

  if (isTestVariantsError) {
    logging.error(testVariantsError);
    throw testVariantsError;
  }

  return (
    <>
      <InvocationCounts
        invocation={invocation}
        testVariants={finalFilteredVariants}
        isLoadingTestVariants={isFetchingTestVariants}
      />
      <Box>
        Loaded {testVariants.length} tests.{' '}
        <Button
          disabled={isLoadingTestVariantsPage || !hasNextPage}
          onClick={() => fetchNextPage()}
          endIcon={
            isLoadingTestVariantsPage ? <CircularProgress size={15} /> : <></>
          }
        >
          {isLoadingTestVariantsPage ? 'Loading' : 'Load more'}
        </Button>
      </Box>
      <Box>
        <TestVariantsTable
          treeData={finalFilteredTree}
          isLoading={isFetchingTestVariants}
          parsedTestId={parsedTestId}
          parsedVariantDef={parsedVariantDef}
          selectedStatuses={selectedStatuses}
          setSelectedStatuses={handleSelectedStatusesChange}
        />
      </Box>
    </>
  );
}
