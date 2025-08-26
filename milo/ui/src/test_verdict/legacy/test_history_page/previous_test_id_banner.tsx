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

import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import { Link as RouterLink } from 'react-router';

import { getTestHistoryURLPath } from '@/common/tools/url_utils';
import { QueryTestMetadataRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { PreviousTestIDHelp } from './previous_test_id_help';
import { useTestMetadata } from './utils';

export interface PreviousTestIdBannerProps {
  readonly project: string;
  readonly testId: string;
}

export function PreviousTestIdBanner({
  project,
  testId,
}: PreviousTestIdBannerProps) {
  const {
    data: testMetadataDetail,
    isSuccess: tmIsSuccess,
    isLoading: tmIsLoading,
  } = useTestMetadata(
    QueryTestMetadataRequest.fromPartial({
      project,
      predicate: { testIds: [testId] },
    }),
  );
  const metadata = testMetadataDetail?.testMetadata;
  const previousTestId = metadata?.previousTestId;
  const previousTestIdHistoryURL = previousTestId
    ? getTestHistoryURLPath(project, previousTestId)
    : null;

  return (
    <>
      {!tmIsLoading &&
        tmIsSuccess &&
        metadata &&
        previousTestId &&
        previousTestIdHistoryURL && (
          <Alert severity="info">
            <Box sx={{ display: 'flex', gap: 1 }}>
              This test has additional history available under a previous ID.
              <PreviousTestIDHelp previousTestId={previousTestId} />
              <Link
                component={RouterLink}
                to={previousTestIdHistoryURL}
                underline="always"
                sx={{ display: 'flex', alignItems: 'center' }}
              >
                View history for previous ID
              </Link>
            </Box>
          </Alert>
        )}
    </>
  );
}
