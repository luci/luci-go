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

import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants';
import { usePrpcQuery } from '@/common/hooks/prpc_query';
import { ResultDb } from '@/common/services/resultdb';

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

  const {
    data: results,
    error,
    isError,
    isLoading,
  } = usePrpcQuery({
    Service: ResultDb,
    host: SETTINGS.resultdb.host,
    method: 'batchGetTestVariants',
    request: {
      invocation: `invocations/${invID}`,
      testVariants: [
        {
          testId: testID!,
          variantHash: vHash,
        },
      ],
    },
  });

  if (isError) {
    throw error;
  }

  if (isLoading) {
    return <LinearProgress />;
  }

  if (!results?.testVariants?.length) {
    throw new Error('No test verdict found matching the provided details.');
  }
  // The variable is called `verdict` even though it is from a test variants
  // array because the test variants array is actually a test verdict array despite its name.
  // We also only expect this array to only contain 1 item.
  const verdict = results.testVariants[0];
  const sources = verdict.sourcesId ? results.sources[verdict.sourcesId] : {};

  return (
    <Grid
      container
      sx={{
        px: 3,
      }}
      rowGap={1}
      flexDirection="column"
    >
      {/** TODO(b/308858986): Format metadata to reflect verdict status. */}
      <PageMeta title="" selectedPage={UiPage.TestVerdict} project={project} />
      <TestVerdictProvider
        invocationID={invID}
        testVerdict={verdict}
        sources={sources}
      >
        <TestIdentifier />
        <VerdictInfo />
        {verdict.results && <TestResults results={verdict.results} />}
      </TestVerdictProvider>
    </Grid>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="test-verdict">
    <TestVerdictPage />
  </RecoverableErrorBoundary>
);
