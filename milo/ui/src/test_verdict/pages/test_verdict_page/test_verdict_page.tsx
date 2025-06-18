// Copyright 2023 The LUCI Authors.
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

import { Divider } from '@mui/material';
import Grid from '@mui/material/Grid2';
import LinearProgress from '@mui/material/LinearProgress';
import { useQuery } from '@tanstack/react-query';
import { upperFirst } from 'lodash-es';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router';

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
import { OutputTestVerdict } from '@/common/types/verdict';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { BatchGetTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { TestVerdictProvider } from './context';
import { TestIdentifier } from './test_identifier';
import { TestResults } from './test_results';
import { VerdictInfo } from './verdict_info';

export function TestVerdictPage() {
  const { project, invID, testID, vHash } = useParams();

  if (!project || !invID || !testID || !vHash) {
    throw new Error(
      'Invariant violated: project, invID, testID, and vHash should be set',
    );
  }
  useEstablishProjectCtx(project);

  const client = useResultDbClient();
  const {
    data: results,
    error,
    isError,
    isPending,
  } = useQuery(
    client.BatchGetTestVariants.query(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: `invocations/${invID}`,
        testVariants: [
          {
            testId: testID!,
            variantHash: vHash,
          },
        ],
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  if (isPending) {
    return <LinearProgress />;
  }

  if (!results.testVariants.length) {
    throw new Error('No test verdict found matching the provided details.');
  }

  // The variable is called `verdict` even though it is from a test variants
  // array because the test variants array is actually a test verdict array
  // despite its name.
  // We also only expect this array to only contain 1 item.
  const verdict = results.testVariants[0] as OutputTestVerdict;
  const sources = verdict.sourcesId ? results.sources[verdict.sourcesId] : null;

  const statusLabel =
    verdict.statusOverride !== TestVerdict_StatusOverride.NOT_OVERRIDDEN
      ? VERDICT_STATUS_OVERRIDE_DISPLAY_MAP[verdict.statusOverride]
      : VERDICT_STATUS_DISPLAY_MAP[verdict.statusV2];

  return (
    <Grid
      container
      sx={{
        px: 1,
        mt: 1,
      }}
      flexDirection="column"
    >
      <Helmet>
        <title>
          {upperFirst(statusLabel)} | {verdict.testId}
        </title>
      </Helmet>
      <TestVerdictProvider
        invocationID={invID}
        project={project}
        testVerdict={verdict}
        sources={sources}
      >
        <TestIdentifier />
        <VerdictInfo />
        <Divider orientation="horizontal" />
        {verdict.results && <TestResults results={verdict.results} />}
      </TestVerdictProvider>
    </Grid>
  );
}

export function Component() {
  useDeclarePageId(UiPage.TestVerdict);

  return (
    <TrackLeafRoutePageView contentGroup="test-verdict">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="test-verdict"
      >
        <TestVerdictPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
