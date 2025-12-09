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
import {
  Box,
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useEffect, useMemo } from 'react';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useEstablishProjectCtx } from '@/common/components/page_meta';
import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants/rpc';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { logging } from '@/common/tools/logging';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { InvocationHeader } from '@/test_investigation/components/invocation_page';
import { InvocationTabs } from '@/test_investigation/components/invocation_page/tabs';
import { RedirectBackBanner } from '@/test_investigation/components/redirect_back_banner';
import { RedirectATIBanner } from '@/test_investigation/components/redirect_back_banner/redirect_ati_banner';
import { InvocationProvider } from '@/test_investigation/context/provider';
import { useInvocationQuery } from '@/test_investigation/hooks/queries';
import { getDisplayInvocationId } from '@/test_investigation/utils/invocation_utils';
import { isAnTSInvocation } from '@/test_investigation/utils/test_info_utils';
import { getProjectFromRealm } from '@/test_investigation/utils/test_variant_utils';

export function InvocationPage() {
  const { invocationId } = useParams<{ invocationId: string }>();
  if (!invocationId) {
    throw new Error('Invalid URL: Missing invocationId');
  }
  const [searchParams] = useSyncedSearchParams();
  const {
    invocation: invocationData,
    isLoading: isLoadingInvocation,
    errors: invocationErrors,
  } = useInvocationQuery(invocationId);

  const invocation = invocationData?.data;
  const isLegacyInvocation = invocationData?.isLegacyInvocation ?? false;

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

  const project = useMemo(
    () => getProjectFromRealm(invocation?.realm),
    [invocation?.realm],
  );

  useEstablishProjectCtx(project);

  useEffect(() => {
    if (invocationErrors.length > 0) {
      invocationErrors.forEach((e) => logging.error(e));
    }
  }, [invocationErrors]);

  if (isLoadingInvocation) {
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

  // Handle all errors based on the loading status

  // 1. If invocation failed to load: log all invocation errors and throw a combined error.
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
    throw new Error(`Invocation data not found for ID: ${invocationId}`);
  }

  return (
    <InvocationProvider
      project={project}
      invocation={invocation}
      rawInvocationId={invocationId!}
      isLegacyInvocation={isLegacyInvocation}
    >
      <ThemeProvider theme={gm3PageTheme}>
        <title>{`Invocation: ${getDisplayInvocationId(invocation)}`}</title>
        {isAnTSInvocation(invocation) ? (
          <RedirectATIBanner invocation={invocation} />
        ) : (
          <RedirectBackBanner
            invocation={invocation}
            parsedTestId={parsedTestId}
            parsedVariantDef={parsedVariantDef}
          />
        )}
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
          <InvocationTabs />
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
