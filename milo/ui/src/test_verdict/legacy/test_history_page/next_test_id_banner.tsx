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

import { Link } from '@mui/material';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import { useMemo } from 'react';
import { Navigate, Link as RouterLink } from 'react-router';

import { getTestHistoryURLWithSearchParam } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { QueryTestMetadataRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { useRedirected } from './hooks';
import { useTestMetadata } from './utils';

export interface NextTestIdBannerProps {
  readonly project: string;
  readonly testId: string;
}

export function NextTestIdBanner({ project, testId }: NextTestIdBannerProps) {
  const { data, isSuccess, isLoading } = useTestMetadata(
    QueryTestMetadataRequest.fromPartial({
      project,
      predicate: { previousTestIds: [testId] },
    }),
  );

  const [searchParams] = useSyncedSearchParams();
  const [redirected] = useRedirected();

  const nextTestId = data?.testId;
  const queryParam = searchParams.get('q') || '';
  const newTestHistoryURL = useMemo(() => {
    return (
      nextTestId &&
      getTestHistoryURLWithSearchParam(project, nextTestId, queryParam, true)
    );
  }, [project, nextTestId, queryParam]);

  if (!isLoading && isSuccess && newTestHistoryURL && !redirected) {
    // Only redirect if the user was not previously redirected. This avoids redirect
    // loops if the project misconfigures the previous_test_id field and if the user
    // deliberately went to the old test ID page from the new one.
    return <Navigate to={newTestHistoryURL} replace />;
  }

  return (
    <>
      {!isLoading && isSuccess && newTestHistoryURL && redirected && (
        <Alert severity="warning">
          <Box sx={{ display: 'flex', gap: 1 }}>
            You are viewing this test using an old ID. Newer results may be
            missing.
            <Link
              component={RouterLink}
              to={newTestHistoryURL}
              underline="always"
              sx={{ display: 'flex', alignItems: 'center' }}
            >
              View test history under current ID
            </Link>
          </Box>
        </Alert>
      )}
    </>
  );
}
