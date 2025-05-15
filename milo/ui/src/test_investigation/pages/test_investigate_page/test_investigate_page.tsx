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

import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import {
  Box,
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useQueries, useQuery } from '@tanstack/react-query';
import { useMemo, JSX } from 'react';
import { useNavigate, useParams } from 'react-router';

import { OutputClusterResponse } from '@/analysis/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useBatchedClustersClient,
  useTestVariantBranchesClient,
  useResultDbClient,
  useAnalysesClient as useLuciBisectionClient,
} from '@/common/hooks/prpc_clients';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  ClusterRequest,
  ClusterResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  SourceRef as AnalysisSourceRef,
  GitilesRef as AnalysisGitilesRef,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { QueryTestVariantBranchRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { BatchGetTestAnalysesRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  GetInvocationRequest,
  BatchGetTestVariantsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestStatus as ResultDbTestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { TestDetails } from '@/test_investigation/components/test_details';
import { TestInfo } from '@/test_investigation/components/test_info';
import { TestNavigationDrawer } from '@/test_investigation/components/test_navigation_drawer';

import { getProjectFromRealm } from '../../utils/test_variant_utils';

export function TestInvestigatePage(): JSX.Element {
  const {
    invocationId: rawInvocationId,
    testId: rawTestId,
    variantHash: rawVariantHash,
  } = useParams<{
    invocationId: string;
    testId: string;
    variantHash: string;
  }>();
  const navigate = useNavigate();

  const resultDbClient = useResultDbClient();
  const analysisClustersClient = useBatchedClustersClient();
  const analysisBranchesClient = useTestVariantBranchesClient();
  const bisectionClient = useLuciBisectionClient();

  const invocationQueryEnabled = !!rawInvocationId;
  const invocationRequest = useMemo(() => {
    if (!invocationQueryEnabled) return GetInvocationRequest.fromPartial({});
    return GetInvocationRequest.fromPartial({
      name: `invocations/${rawInvocationId}`,
    });
  }, [invocationQueryEnabled, rawInvocationId]);

  const {
    data: invocationData,
    isPending: isLoadingInvocation,
    isError: isInvocationError,
    error: invocationError,
  } = useQuery({
    ...resultDbClient.GetInvocation.query(invocationRequest),
    enabled: invocationQueryEnabled,
    staleTime: 5 * 60 * 1000,
    select: (data: Invocation | null) => data,
  });

  const decodedTestId = useMemo(
    () => (rawTestId ? decodeURIComponent(rawTestId) : undefined),
    [rawTestId],
  );

  const testVariantQueryEnabled = !!(
    rawInvocationId &&
    decodedTestId &&
    rawVariantHash
  );
  const testVariantRequest = useMemo(() => {
    if (!testVariantQueryEnabled) {
      return BatchGetTestVariantsRequest.fromPartial({});
    }
    return BatchGetTestVariantsRequest.fromPartial({
      invocation: `invocations/${rawInvocationId}`,
      testVariants: [
        {
          testId: decodedTestId,
          variantHash: rawVariantHash,
        },
      ],
      resultLimit: 100,
    });
  }, [testVariantQueryEnabled, rawInvocationId, decodedTestId, rawVariantHash]);

  const {
    data: testVariantData,
    isPending: isLoadingTestVariant,
    isError: isTestVariantError,
    error: testVariantError,
  } = useQuery({
    ...resultDbClient.BatchGetTestVariants.query(testVariantRequest),
    enabled: testVariantQueryEnabled,
    staleTime: Infinity,
    select: (data): TestVariant | null => {
      if (!data || !data.testVariants || data.testVariants.length === 0)
        return null;
      return data.testVariants[0];
    },
  });

  const project = useMemo(
    () => getProjectFromRealm(invocationData?.realm),
    [invocationData?.realm],
  );

  const resultsToCluster = useMemo(() => {
    if (!testVariantData || !testVariantData.results) return [];
    return testVariantData.results
      .map((rlink) => rlink.result)
      .filter(
        (r) =>
          r &&
          !r.expected &&
          r.status &&
          ![ResultDbTestStatus.PASS, ResultDbTestStatus.SKIP].includes(
            r.status,
          ),
      );
  }, [testVariantData]);

  const associatedBugsQueriesEnabled = !!(
    project &&
    resultsToCluster.length > 0 &&
    testVariantData?.testId
  );

  const associatedBugsQueries = useQueries({
    queries: associatedBugsQueriesEnabled
      ? resultsToCluster.map((r) => ({
          ...analysisClustersClient.Cluster.query(
            ClusterRequest.fromPartial({
              project,
              testResults: [
                {
                  testId: testVariantData!.testId,
                  failureReason: r!.failureReason,
                },
              ],
            }),
          ),
          select: (res: ClusterResponse): OutputClusterResponse =>
            res as OutputClusterResponse,
          enabled: associatedBugsQueriesEnabled,
          staleTime: 5 * 60 * 1000,
        }))
      : [],
  });

  const isLoadingAssociatedBugs =
    associatedBugsQueriesEnabled &&
    associatedBugsQueries.some((q) => q.isPending);
  const associatedBugsFetchErrorQuery =
    associatedBugsQueriesEnabled &&
    associatedBugsQueries.find((q) => q.isError);
  const associatedBugsFetchError: string | undefined =
    associatedBugsFetchErrorQuery
      ? (associatedBugsFetchErrorQuery?.error as unknown as string)
      : undefined;

  const associatedBugs: AssociatedBug[] = useMemo(() => {
    if (
      !associatedBugsQueriesEnabled ||
      associatedBugsQueries.some((q) => !q.data || q.isError)
    ) {
      return [];
    }
    const bugs = associatedBugsQueries
      .flatMap((q) =>
        q.data!.clusteredTestResults.flatMap((ctr) =>
          ctr.clusters.map((c) => c.bug),
        ),
      )
      .filter((bug) => bug !== undefined && bug !== null) as AssociatedBug[];

    const uniqueBugs = new Map<string, AssociatedBug>();
    bugs.forEach((bug) => {
      const key = `${bug.system}-${bug.id}`;
      if (!uniqueBugs.has(key)) {
        uniqueBugs.set(key, bug);
      }
    });
    return Array.from(uniqueBugs.values());
  }, [associatedBugsQueriesEnabled, associatedBugsQueries]);

  const sourceRefForAnalysis: AnalysisSourceRef | undefined = useMemo(() => {
    const gc = invocationData?.sourceSpec?.sources?.gitilesCommit;
    if (gc?.host && gc.project && gc.ref) {
      return AnalysisSourceRef.fromPartial({
        gitiles: AnalysisGitilesRef.fromPartial({
          host: gc.host,
          project: gc.project,
          ref: gc.ref,
        }),
      });
    }
    return undefined;
  }, [invocationData?.sourceSpec?.sources?.gitilesCommit]);

  const testVariantBranchQueryEnabled = !!(
    project &&
    testVariantData?.testId &&
    sourceRefForAnalysis &&
    testVariantData?.variantHash
  );

  const testVariantBranchRequest = useMemo(() => {
    if (!testVariantBranchQueryEnabled)
      return QueryTestVariantBranchRequest.fromPartial({});
    return QueryTestVariantBranchRequest.fromPartial({
      project: project!,
      testId: testVariantData!.testId,
      ref: sourceRefForAnalysis!,
    });
  }, [
    testVariantBranchQueryEnabled,
    project,
    testVariantData,
    sourceRefForAnalysis,
  ]);

  const {
    data: testVariantBranchData,
    isPending: isLoadingTestVariantBranch,
    isError: isTestVariantBranchError,
    error: testVariantBranchError,
  } = useQuery({
    ...analysisBranchesClient.Query.query(testVariantBranchRequest),
    enabled: testVariantBranchQueryEnabled,
    staleTime: 5 * 60 * 1000,
    select: (response) => {
      return (
        response.testVariantBranch?.find(
          (tvb) => tvb.variantHash === testVariantData!.variantHash,
        ) || null
      );
    },
  });

  const bisectionAnalysisQueryEnabled = !!(
    project &&
    testVariantData?.testId &&
    testVariantData?.variantHash &&
    testVariantBranchData?.refHash
  );

  const bisectionTestFailureIdentifier = useMemo(() => {
    if (!bisectionAnalysisQueryEnabled) return undefined;
    return {
      testId: testVariantData!.testId,
      variantHash: testVariantData!.variantHash!,
      refHash: testVariantBranchData!.refHash!,
    };
  }, [bisectionAnalysisQueryEnabled, testVariantData, testVariantBranchData]);

  const bisectionAnalysisRequest = useMemo(() => {
    if (!bisectionAnalysisQueryEnabled || !bisectionTestFailureIdentifier) {
      return BatchGetTestAnalysesRequest.fromPartial({});
    }
    return BatchGetTestAnalysesRequest.fromPartial({
      project: project!,
      testFailures: [bisectionTestFailureIdentifier],
    });
  }, [bisectionAnalysisQueryEnabled, project, bisectionTestFailureIdentifier]);

  const {
    data: bisectionAnalysisData,
    isPending: isLoadingBisectionAnalysis,
    isError: isBisectionAnalysisError,
    error: bisectionAnalysisError,
  } = useQuery({
    ...bisectionClient.BatchGetTestAnalyses.query(bisectionAnalysisRequest),
    enabled: bisectionAnalysisQueryEnabled,
    staleTime: 15 * 60 * 1000, // More stale as bisection results change less often
    select: (response) => {
      return response.testAnalyses && response.testAnalyses.length > 0
        ? response.testAnalyses[0]
        : null;
    },
  });

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

  if (!rawInvocationId || !rawTestId || !rawVariantHash) {
    const missingParams: string[] = [];
    if (!rawInvocationId) missingParams.push('Invocation ID');
    if (!rawTestId) missingParams.push('Test ID');
    if (!rawVariantHash) missingParams.push('Variant Hash');
    const message = `The following URL parameter(s) are required: ${missingParams.join(', ')}.`;
    return (
      <Box
        sx={{
          p: 4,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: 'calc(100vh - 64px)',
          textAlign: 'center',
        }}
      >
        <ErrorOutlineIcon color="error" sx={{ fontSize: '4rem', mb: 2 }} />
        <Typography variant="h5" color="error" gutterBottom>
          Missing Required Information
        </Typography>
        <Typography variant="body1" color="text.secondary">
          {message}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          Please ensure the URL is complete. Example:
          /ui/test-investigate/invocations/:invId/tests/:testId/variants/:variantHash
        </Typography>
      </Box>
    );
  }

  const isAnythingLoading =
    isLoadingInvocation ||
    isLoadingTestVariant ||
    isLoadingAssociatedBugs ||
    isLoadingTestVariantBranch ||
    isLoadingBisectionAnalysis;

  if (isAnythingLoading) {
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
        <Typography sx={{ ml: 2 }}>
          Loading test investigation data...
        </Typography>
      </Box>
    );
  }

  const errorChecks = [
    {
      isError: isInvocationError,
      error: invocationError,
      name: 'Invocation Data',
    },
    {
      isError: isTestVariantError,
      error: testVariantError,
      name: 'Test Variant Data',
    },
    {
      isError: !!associatedBugsFetchError,
      error: associatedBugsFetchError,
      name: 'Associated Bugs',
    },
    {
      isError: isTestVariantBranchError,
      error: testVariantBranchError,
      name: 'Test History',
    },
    {
      isError: isBisectionAnalysisError,
      error: bisectionAnalysisError,
      name: 'Bisection Analysis',
    },
  ];

  for (const check of errorChecks) {
    if (check.isError) {
      return (
        <Box
          sx={{
            p: 2,
            textAlign: 'center',
            height: '100vh',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center',
            alignItems: 'center',
          }}
        >
          <ErrorOutlineIcon color="error" sx={{ fontSize: '3rem', mb: 1 }} />
          <Typography variant="h6" color="error">
            Error Loading {check.name}
          </Typography>
          <Typography color="text.secondary">
            {check.error instanceof Error
              ? check.error.message
              : 'An unknown error occurred.'}
          </Typography>
        </Box>
      );
    }
  }

  if (!invocationData) {
    return (
      <Box
        sx={{
          p: 2,
          textAlign: 'center',
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          alignItems: 'center',
        }}
      >
        <ErrorOutlineIcon color="action" sx={{ fontSize: '3rem', mb: 1 }} />
        <Typography variant="h6">Invocation Not Found</Typography>
        <Typography color="text.secondary">
          No invocation data was found for ID: {rawInvocationId}.
        </Typography>
      </Box>
    );
  }
  if (!testVariantData) {
    return (
      <Box
        sx={{
          p: 2,
          textAlign: 'center',
          height: '100vh',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          alignItems: 'center',
        }}
      >
        <ErrorOutlineIcon color="action" sx={{ fontSize: '3rem', mb: 1 }} />
        <Typography variant="h6">Test Variant Not Found</Typography>
        <Typography color="text.secondary">
          No test variant data was found for the specified parameters.
        </Typography>
      </Box>
    );
  }

  return (
    <ThemeProvider theme={gm3PageTheme}>
      <Box sx={{ position: 'relative', height: '100vh', overflowY: 'auto' }}>
        <Box component="main" sx={{ height: '100%' }}>
          <Box
            sx={{
              padding: { xs: 1, sm: 2 },
              display: 'flex',
              flexDirection: 'column',
              gap: { xs: '24px', md: '36px' },
              maxWidth: '100%',
              boxSizing: 'border-box',
            }}
          >
            <TestInfo
              invocation={invocationData}
              testVariant={testVariantData}
              associatedBugs={associatedBugs}
              testVariantBranch={testVariantBranchData}
              bisectionAnalysis={bisectionAnalysisData}
            />
            <TestDetails
              invocation={invocationData}
              testVariant={testVariantData}
            />
          </Box>
        </Box>

        {invocationData && (
          <TestNavigationDrawer
            invocation={invocationData}
            onSelectTestVariant={handleDrawerTestSelection}
            currentTestId={decodedTestId}
            currentVariantHash={rawVariantHash}
          />
        )}
      </Box>
    </ThemeProvider>
  );
}

export function Component(): JSX.Element {
  return (
    <TrackLeafRoutePageView contentGroup="test-investigate">
      <RecoverableErrorBoundary key="test-investigate">
        <TestInvestigatePage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
