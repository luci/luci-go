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

import { GrpcError } from '@chopsui/prpc-client';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import {
  Box,
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { startCase } from 'lodash-es';
import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router';

import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants/rpc';
import {
  VERDICT_STATUS_DISPLAY_MAP,
  VERDICT_STATUS_OVERRIDE_DISPLAY_MAP,
} from '@/common/constants/verdict';
import { UiPage } from '@/common/constants/view';
import { useFeatureFlag } from '@/common/feature_flags/context';
import {
  useResultDbClient,
  useTestHistoryClient,
} from '@/common/hooks/prpc_clients';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { logging } from '@/common/tools/logging';
import { OutputTestVerdict } from '@/common/types/verdict';
import { requestSurvey } from '@/fleet/utils/survey';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { QueryRecentPassesRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import {
  TestIdentifier,
  Sources,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { BatchGetTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { ArtifactsSection } from '@/test_investigation/components/artifacts/artifacts_section';
import { RedirectBackBanner } from '@/test_investigation/components/redirect_back_banner';
import { RedirectATIBanner } from '@/test_investigation/components/redirect_back_banner/redirect_ati_banner';
import { TestInfo } from '@/test_investigation/components/test_info';
import { TestNavigationDrawer } from '@/test_investigation/components/test_navigation_drawer';
import { TestDrawerProvider } from '@/test_investigation/components/test_navigation_drawer/context';
import {
  InvocationProvider,
  RecentPassesProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { useInvocationQuery } from '@/test_investigation/hooks/queries';
import { USE_ROOT_INVOCATION_FLAG } from '@/test_investigation/pages/features';
import {
  isAnTSInvocation,
  isPresubmitRun,
} from '@/test_investigation/utils/test_info_utils';
import { getProjectFromRealm } from '@/test_investigation/utils/test_variant_utils';

// Define the shape of the URL parameters.
// `coarse` and `fine` will be undefined if not in the URL.
type TestInvestigateParams = {
  invocationId: string;
  module: string;
  scheme: string;
  variant: string;
  coarse?: string;
  fine?: string;
  case: string;
};

export function TestInvestigatePage() {
  const params = useParams<TestInvestigateParams>();

  const authState = useAuthState();
  const resultDbClient = useResultDbClient();
  const testHistoryClient = useTestHistoryClient();

  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  const handleToggleDrawer = () => {
    setIsDrawerOpen((prev) => !prev);
  };

  // Destructure all params needed for both legacy and root invocations.
  const {
    invocationId: rawInvocationId,
    variant: rawVariantHash,
    case: decodedTestId, // The router already decodes the 'case' parameter
    module,
    scheme,
    coarse,
    fine,
  } = params;

  if (
    !rawInvocationId ||
    !rawVariantHash ||
    !decodedTestId ||
    !module ||
    !scheme
  ) {
    throw new Error('Invalid URL: Missing required parameters.');
  }

  // Fetch Legacy Invocation (only if in legacy mode)
  const {
    invocation: invocationData,
    isLoading: isLoadingInvocation,
    errors: invocationErrors,
  } = useInvocationQuery(rawInvocationId);

  const invocation = invocationData?.data;
  const isLegacyInvocation = invocationData?.isLegacyInvocation ?? false;

  const invocationNameForTestVariants = isLegacyInvocation
    ? `invocations/${rawInvocationId}`
    : `rootInvocations/${rawInvocationId}`;

  const request = useMemo(() => {
    if (module === 'legacy') {
      return BatchGetTestVariantsRequest.fromPartial({
        invocation: invocationNameForTestVariants,
        testVariants: [
          {
            testId: decodedTestId,
            variantHash: rawVariantHash,
          },
        ],
        resultLimit: 100,
      });
    } else {
      return BatchGetTestVariantsRequest.fromPartial({
        parent: invocationNameForTestVariants,
        testVariants: [
          {
            testIdStructured: TestIdentifier.fromPartial({
              moduleName: module,
              moduleScheme: scheme,
              moduleVariantHash: rawVariantHash,
              caseName: decodedTestId,
              coarseName: coarse,
              fineName: fine,
            }),
          },
        ],
        resultLimit: 100,
      });
    }
  }, [
    invocationNameForTestVariants,
    decodedTestId,
    rawVariantHash,
    module,
    scheme,
    coarse,
    fine,
  ]);

  const {
    data: testVariantData,
    isPending: isLoadingTestVariant,
    error: testVariantError,
  } = useQuery({
    ...resultDbClient.BatchGetTestVariants.query(request),
    staleTime: Infinity,
    select: (
      data,
    ): { testVariant: OutputTestVerdict; sources?: Sources } | null => {
      if (!data?.testVariants?.length) {
        return null;
      }
      const tv = data.testVariants[0] as OutputTestVerdict;
      const sources = tv.sourcesId ? data.sources[tv.sourcesId] : undefined;
      return { testVariant: tv, sources };
    },
    enabled: !!request && !!invocation,
    placeholderData: keepPreviousData,
  });

  if (
    testVariantError instanceof GrpcError &&
    POTENTIAL_PERM_ERROR_CODES.includes(testVariantError.code)
  ) {
    throw testVariantError;
  }

  const testVariant = testVariantData?.testVariant;
  const sources = testVariantData?.sources;

  const project = useMemo(
    () => getProjectFromRealm(invocation?.realm),
    [invocation?.realm],
  );

  const { data: recentPasses, error: recentPassesError } = useQuery({
    ...testHistoryClient.QueryRecentPasses.query(
      QueryRecentPassesRequest.fromPartial({
        project: project,
        testId: testVariant?.testId,
        variantHash: rawVariantHash,
        sources: sources,
      }),
    ),
    staleTime: 5 * 60 * 1000,
    enabled:
      !!project && !!testVariant?.testId && !!rawVariantHash && !!sources,
  });

  const displayStatusString = useMemo(() => {
    if (testVariant) {
      const statusString =
        testVariant.statusOverride !== TestVerdict_StatusOverride.NOT_OVERRIDDEN
          ? VERDICT_STATUS_OVERRIDE_DISPLAY_MAP[testVariant.statusOverride]
          : VERDICT_STATUS_DISPLAY_MAP[testVariant.statusV2];
      return startCase(statusString);
    }
    return '';
  }, [testVariant]);

  useEstablishProjectCtx(project);
  useDeclarePageId(UiPage.TestInvestigation);

  const { trackEvent } = useGoogleAnalytics();

  // Send an analytics event when the invocation and test variant data are loaded.
  useEffect(() => {
    if (invocation && testVariant && project) {
      const invocationType = isPresubmitRun(invocation)
        ? 'presubmit'
        : 'postsubmit';
      trackEvent('test_investigate_page_loaded', {
        project,
        invocationType,
      });
    }
  }, [invocation, testVariant, project, trackEvent]);

  useEffect(() => {
    // Trigger HaTs survey on first page load.
    if (SETTINGS.testInvestigate?.hatsCuj) {
      requestSurvey(SETTINGS.testInvestigate.hatsCuj, authState);
    }
  }, [authState]);

  useEffect(() => {
    if (invocationErrors.length > 0) {
      invocationErrors.forEach((e) => logging.error(e));
    }
  }, [invocationErrors]);

  if (isLoadingInvocation || (invocation && isLoadingTestVariant)) {
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

  if (!invocation) {
    // Check if any error is a permission error and throw it directly to trigger the login prompt.
    for (const error of invocationErrors) {
      if (
        error instanceof GrpcError &&
        POTENTIAL_PERM_ERROR_CODES.includes(error.code)
      ) {
        throw error;
      }
    }

    const errorMessages = invocationErrors
      .map((e) => (e instanceof Error ? e.message : String(e)))
      .join('; ');

    if (invocationErrors.length > 0) {
      throw new Error(`Failed to load invocation: ${errorMessages}`);
    }

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
  if (!testVariant) {
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
          No test variant data was found for Test ID: {decodedTestId}, Variant
          Hash: {rawVariantHash}.
        </Typography>
      </Box>
    );
  }

  // TODO(b/463488897): ATI redirect banner should link directly to Android test result page instead of just invocation.
  return (
    <InvocationProvider
      invocation={invocation}
      rawInvocationId={rawInvocationId}
      project={project}
      isLegacyInvocation={isLegacyInvocation}
    >
      {/** TODO: Add favicon */}
      <title>
        {`${displayStatusString} - ${' '}
        ${testVariant.testMetadata?.name || testVariant.testId} - Test
        Investigation`}
      </title>
      <TestVariantProvider
        testVariant={testVariant}
        displayStatusString={displayStatusString}
      >
        <RecentPassesProvider
          passingResults={recentPasses?.passingResults}
          error={recentPassesError}
        >
          <ThemeProvider theme={gm3PageTheme}>
            {isAnTSInvocation(invocation) ? (
              <RedirectATIBanner invocation={invocation} />
            ) : (
              <RedirectBackBanner
                invocation={invocation}
                testVariant={testVariant}
              />
            )}
            <Box component="main" sx={{ height: '100%' }}>
              <Box
                sx={{
                  padding: { xs: 1, sm: 2 },
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 3,
                  maxWidth: `calc(100vw - 16px)`,
                  boxSizing: 'border-box',
                  height: '100%',
                }}
              >
                <TestInfo />
                <ArtifactsSection />
              </Box>
            </Box>
            <TestDrawerProvider isDrawerOpen={isDrawerOpen}>
              <TestNavigationDrawer
                isOpen={isDrawerOpen}
                onToggleDrawer={handleToggleDrawer}
              />
            </TestDrawerProvider>
          </ThemeProvider>
        </RecentPassesProvider>
      </TestVariantProvider>
    </InvocationProvider>
  );
}

export function Component() {
  const useRootInvocation = useFeatureFlag(USE_ROOT_INVOCATION_FLAG);

  return (
    <TrackLeafRoutePageView contentGroup="test-investigate">
      <RecoverableErrorBoundary
        key="test-investigate"
        resetKeys={[useRootInvocation]}
      >
        <TestInvestigatePage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
