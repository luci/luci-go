import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import {
  Box,
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { startCase } from 'lodash-es';
import { useEffect, useMemo } from 'react';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router';

import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  useDeclarePageId,
  useEstablishProjectCtx,
} from '@/common/components/page_meta';
import {
  VERDICT_STATUS_DISPLAY_MAP,
  VERDICT_STATUS_OVERRIDE_DISPLAY_MAP,
} from '@/common/constants/verdict';
import { UiPage } from '@/common/constants/view';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { OutputTestVerdict } from '@/common/types/verdict';
import { requestSurvey } from '@/fleet/utils/survey';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { TestIdentifier } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import {
  GetInvocationRequest,
  BatchGetTestVariantsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { GetRootInvocationRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { ArtifactsSection } from '@/test_investigation/components/artifacts/artifacts_section';
import { RedirectBackBanner } from '@/test_investigation/components/redirect_back_banner';
import { TestInfo } from '@/test_investigation/components/test_info';
import { TestNavigationDrawer } from '@/test_investigation/components/test_navigation_drawer';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context/provider';
import { isPresubmitRun } from '@/test_investigation/utils/test_info_utils';
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
  const [searchParams] = useSyncedSearchParams();
  const isLegacyInvocation = searchParams.get('invMode') === 'legacy';

  const authState = useAuthState();
  const resultDbClient = useResultDbClient();

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
  const { data: legacyInvocation, isPending: isLoadingLegacyInv } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: `invocations/${rawInvocationId}`,
      }),
    ),
    staleTime: 5 * 60 * 1000,
    enabled: isLegacyInvocation,
  });

  // Fetch Root Invocation (only if NOT in legacy mode)
  const { data: rootInvocation, isPending: isLoadingRootInv } = useQuery({
    ...resultDbClient.GetRootInvocation.query(
      GetRootInvocationRequest.fromPartial({
        name: `rootInvocations/${rawInvocationId}`,
      }),
    ),
    staleTime: 5 * 60 * 1000,
    enabled: !isLegacyInvocation,
  });

  const invocation = isLegacyInvocation ? legacyInvocation : rootInvocation;
  const isLoadingInvocation = isLegacyInvocation
    ? isLoadingLegacyInv
    : isLoadingRootInv;

  const invocationNameForTestVariants = isLegacyInvocation
    ? `invocations/${rawInvocationId}`
    : `rootInvocations/${rawInvocationId}`;

  const request = useMemo(() => {
    if (isLegacyInvocation) {
      // For legacy invocations, query by testId and variantHash.
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
      // For root invocations, query by the structured test identifier.
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
    isLegacyInvocation,
    invocationNameForTestVariants,
    decodedTestId,
    rawVariantHash,
    module,
    scheme,
    coarse,
    fine,
  ]);

  const { data: testVariant, isPending: isLoadingTestVariant } = useQuery({
    ...resultDbClient.BatchGetTestVariants.query(request),
    staleTime: Infinity,
    select: (data): OutputTestVerdict | null => {
      if (!data?.testVariants?.length) {
        return null;
      }
      return data.testVariants[0] as OutputTestVerdict;
    },
    // Only enable the query once the request is fully formed.
    enabled: !!request,
  });

  const project = useMemo(
    () => getProjectFromRealm(invocation?.realm),
    [invocation?.realm],
  );

  // Prepare the display string, e.g., "Failed" instead of "UNEXPECTED"
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

  if (isLoadingInvocation || isLoadingTestVariant) {
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

  return (
    <InvocationProvider
      invocation={invocation}
      rawInvocationId={rawInvocationId}
      project={project}
      isLegacyInvocation={isLegacyInvocation}
    >
      <Helmet>
        {/** TODO: Add favicon */}
        <title>
          {displayStatusString} -{' '}
          {testVariant.testMetadata?.name || testVariant.testId} - Test
          Investigation
        </title>
      </Helmet>
      <TestVariantProvider
        testVariant={testVariant}
        displayStatusString={displayStatusString}
      >
        <ThemeProvider theme={gm3PageTheme}>
          <RedirectBackBanner
            invocation={invocation}
            testVariant={testVariant}
          />
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
          <TestNavigationDrawer />
        </ThemeProvider>
      </TestVariantProvider>
    </InvocationProvider>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="test-investigate">
      <RecoverableErrorBoundary key="test-investigate">
        <TestInvestigatePage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
