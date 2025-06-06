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
  CircularProgress,
  ThemeProvider,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { gm3PageTheme } from '@/common/themes/gm3_theme';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  GetInvocationRequest,
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  InvocationCounts,
  InvocationHeader,
  TestVariantsTable,
} from '@/test_investigation/components/invocation_page';
import { InvocationProvider } from '@/test_investigation/context/provider';

export function InvocationPage() {
  const { invocationId: invocationId } = useParams<{
    invocationId: string;
  }>();

  const resultDbClient = useResultDbClient();

  if (!invocationId) {
    throw new Error('Invalid URL: Missing invocationId');
  }

  const { data: invocation, isPending: isLoadingInvocation } = useQuery({
    ...resultDbClient.GetInvocation.query(
      GetInvocationRequest.fromPartial({
        name: `invocations/${invocationId}`,
      }),
    ),
    retry: false,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });

  // Query to fetch all interesting test variants.
  const {
    data: testVariantsResponse,
    isPending: isLoadingTestVariants,
    error,
    isError,
  } = useQuery<QueryTestVariantsResponse | null, Error, readonly TestVariant[]>(
    {
      ...resultDbClient.QueryTestVariants.query(
        QueryTestVariantsRequest.fromPartial({
          invocations: [`invocations/${invocationId}`],
          // TODO: Implement pagination for invocations with >1000 test variants.
          pageSize: 1000,
          resultLimit: 10,
          readMask: [
            'test_id',
            'test_id_structured',
            'variant_hash',
            'variant.def',
            'status',
            'status_v2',
            'results.*.result.status_v2',
            'results.*.result.failure_reason',
          ],
          orderBy: 'status_v2_effective',
        }),
      ),
      staleTime:
        invocation?.state === Invocation_State.FINALIZED ? 0 : 5 * 60 * 1000,
      select: (data) => data?.testVariants || [],
    },
  );

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
        <Typography sx={{ ml: 2 }}>Loading invocation data...</Typography>
      </Box>
    );
  }

  if (isError || !invocation) {
    throw error;
  }

  return (
    <InvocationProvider invocation={invocation} rawInvocationId={invocationId}>
      <ThemeProvider theme={gm3PageTheme}>
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
            testVariants={testVariantsResponse}
            isLoadingTestVariants={isLoadingTestVariants}
          />
          <Box sx={{ mt: '24px' }}>
            <TestVariantsTable testVariants={testVariantsResponse || []} />
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
