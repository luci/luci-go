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

import { FormControlLabel, FormGroup, Switch } from '@mui/material';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';

import { QueryTestMetadataRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { useFollowRenames } from './hooks';
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
  const [followRenames, setFollowRenames] = useFollowRenames();

  return (
    <>
      {!tmIsLoading && tmIsSuccess && metadata && previousTestId && (
        <Alert severity="info">
          <Box sx={{ display: 'flex', gap: 1 }}>
            This test has additional history under a previous ID.
            <PreviousTestIDHelp
              project={project}
              previousTestId={previousTestId}
            />
            <FormGroup sx={{ marginLeft: 2 }}>
              <FormControlLabel
                control={
                  <Switch
                    size="small"
                    checked={followRenames}
                    onClick={() => setFollowRenames(!followRenames)}
                  />
                }
                label="Include history from previous test ID"
                disableTypography={true}
                sx={{ marginTop: '-2px', marginBottom: '-2px' }}
              />
            </FormGroup>
          </Box>
        </Alert>
      )}
    </>
  );
}
