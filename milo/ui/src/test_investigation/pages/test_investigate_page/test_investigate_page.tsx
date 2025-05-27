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
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  GetInvocationRequest,
  BatchGetTestVariantsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { TestDetails } from '@/test_investigation/components/test_details';
import { TestInfo } from '@/test_investigation/components/test_info';
import { TestNavigationDrawer } from '@/test_investigation/components/test_navigation_drawer';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context/provider';

export function TestInvestigatePage() {
  const {
    invocationId: rawInvocationId,
    testId: rawTestId,
    variantHash: rawVariantHash,
  } = useParams<{
    invocationId: string;
    testId: string;
    variantHash: string;
  }>();

  const resultDbClient = useResultDbClient();

  if (!rawInvocationId || !rawTestId || !rawVariantHash) {
    throw new Error(
      'Invalid URL: Missing invocationId, testId, or variantHash.',
    );
  }

  const decodedTestId = useMemo(
    () => decodeURIComponent(rawTestId),
    [rawTestId],
  );

  const { data: invocation, isPending: isLoadingInvocation } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: `invocations/${rawInvocationId}`,
      }),
    ),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });

  const { data: testVariant, isPending: isLoadingTestVariant } = useQuery({
    ...resultDbClient.BatchGetTestVariants.query(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: `invocations/${rawInvocationId}`,
        testVariants: [
          {
            testId: decodedTestId,
            variantHash: rawVariantHash,
          },
        ],
        resultLimit: 100,
      }),
    ),
    staleTime: Infinity, // TestVariant data for a specific inv and hash is usually immutable
    select: (data): TestVariant | null => {
      if (!data || !data.testVariants || data.testVariants.length === 0) {
        return null;
      }
      return data.testVariants[0];
    },
  });

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
    >
      <TestVariantProvider testVariant={testVariant}>
        <ThemeProvider theme={gm3PageTheme}>
          <Box
            sx={{ position: 'relative', height: '100vh', overflowY: 'auto' }}
          >
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
                <TestInfo />
                <TestDetails />
              </Box>
            </Box>
            <TestNavigationDrawer />
          </Box>
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
