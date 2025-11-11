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
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState, useRef } from 'react';
import { useNavigate, useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useEstablishProjectCtx } from '@/common/components/page_meta';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  semanticStatusForTestVariant,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import {
  generateTestInvestigateUrl,
  generateTestInvestigateUrlForLegacyInvocations,
} from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  GetInvocationRequest,
  QueryTestVariantsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { GetRootInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import {
  InvocationCounts,
  InvocationHeader,
  TestVariantsTable,
} from '@/test_investigation/components/invocation_page';
import { RedirectBackBanner } from '@/test_investigation/components/redirect_back_banner';
import { TestNavigationTreeNode } from '@/test_investigation/components/test_navigation_drawer/types';
import { InvocationProvider } from '@/test_investigation/context/provider';
import { buildHierarchyTree } from '@/test_investigation/utils/drawer_tree_utils';
import { getDisplayInvocationId } from '@/test_investigation/utils/invocation_utils';
import { getProjectFromRealm } from '@/test_investigation/utils/test_variant_utils';

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

export function InvocationPage() {
  const { invocationId } = useParams<{ invocationId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSyncedSearchParams();
  const hasPerformedInitialRedirect = useRef(false);
  const resultDbClient = useResultDbClient();

  const isLegacyInvocation = searchParams.get('invMode') === 'legacy';

  if (!invocationId) {
    throw new Error('Invalid URL: Missing invocationId');
  }

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
    // Initialize with default filters only no other filters are set.
    if (parsedTestId || parsedVariantDef) {
      return new Set();
    }
    return new Set(['failed', 'execution_errored']);
  });

  const { data: legacyInvocation, isPending: isLoadingLegacyInv } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: `invocations/${invocationId}`,
      }),
    ),
    retry: false,
    staleTime: 5 * 60 * 1000,
    enabled: isLegacyInvocation,
  });

  const { data: rootInvocation, isPending: isLoadingRootInv } = useQuery({
    ...resultDbClient.GetRootInvocation.query(
      GetRootInvocationRequest.fromPartial({
        name: `rootInvocations/${invocationId}`,
      }),
    ),
    retry: false,
    staleTime: 5 * 60 * 1000,
    enabled: !isLegacyInvocation,
  });

  const invocation = isLegacyInvocation ? legacyInvocation : rootInvocation;
  const isLoadingInvocation = isLegacyInvocation
    ? isLoadingLegacyInv
    : isLoadingRootInv;

  const queryRequest = useMemo(() => {
    const filterParts: string[] = [];
    if (parsedTestId) {
      // The proto doc says a bare value is searched in test_id. Using an
      // explicit `test_id:` with the "has" operator for a substring search.
      filterParts.push(`test_id:${JSON.stringify(parsedTestId)}`);
    }
    if (parsedVariantDef) {
      Object.entries(parsedVariantDef).forEach(([key, value]) => {
        // The example `-variant.os:mac` in the proto suggests that filtering
        // on variant key-value pairs is supported. We use `=` for an exact
        // match on the value.
        filterParts.push(`variant.${key} = ${JSON.stringify(value)}`);
      });
    }
    const filter = filterParts.join(' AND ') || undefined;

    const readMask = [
      'test_id',
      'test_id_structured',
      'variant_hash',
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

    if (isLegacyInvocation) {
      return QueryTestVariantsRequest.fromPartial({
        invocations: [`invocations/${invocationId}`],
        pageSize,
        resultLimit,
        readMask,
        filter,
      });
    } else {
      return QueryTestVariantsRequest.fromPartial({
        parent: `rootInvocations/${invocationId}`,
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
    error,
    isError,
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
      let newPath: string;

      if (isLegacyInvocation) {
        newPath = generateTestInvestigateUrlForLegacyInvocations(
          invocationId,
          variantToRedirect.testId,
          variantToRedirect.variantHash,
        );
      } else {
        // We are on a RootInvocation, generate the new structured URL.
        if (variantToRedirect.testIdStructured) {
          newPath = generateTestInvestigateUrl(
            invocationId,
            variantToRedirect.testIdStructured,
          );
        } else {
          // Fallback to legacy structure just in case
          newPath = generateTestInvestigateUrlForLegacyInvocations(
            invocationId,
            variantToRedirect.testId,
            variantToRedirect.variantHash,
          );
        }
      }
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

  const project = useMemo(
    () => getProjectFromRealm(invocation?.realm),
    [invocation?.realm],
  );

  useEstablishProjectCtx(project);

  const showInitialLoader =
    isFetchingTestVariants && !hasPerformedInitialRedirect.current;
  const isRedirecting = isUniqueResult && !hasPerformedInitialRedirect.current;

  if (isLoadingInvocation || showInitialLoader || isRedirecting) {
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

  if (isError || !invocation) {
    throw error;
  }

  return (
    <InvocationProvider
      project={project}
      invocation={invocation}
      rawInvocationId={invocationId}
      isLegacyInvocation={isLegacyInvocation}
    >
      <ThemeProvider theme={gm3PageTheme}>
        <title>{`Invocation: ${getDisplayInvocationId(invocation)}`}</title>
        <RedirectBackBanner
          invocation={invocation}
          parsedTestId={parsedTestId}
          parsedVariantDef={parsedVariantDef}
        />

        <Box
          component="main"
          sx={{
            padding: { xs: 2, sm: 3 },
            display: 'flex',
            flexDirection: 'column',
            gap: '8px',
          }}
        >
          <InvocationHeader invocation={invocation} />
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
                isLoadingTestVariantsPage ? (
                  <CircularProgress size={15} />
                ) : (
                  <></>
                )
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
              setSelectedStatuses={setSelectedStatuses}
              isLegacyInvocation={isLegacyInvocation}
            />
          </Box>
        </Box>
      </ThemeProvider>
    </InvocationProvider>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="invocation-overview">
      <RecoverableErrorBoundary key="invocation-overview">
        <InvocationPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
