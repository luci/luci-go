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

import Divider from '@mui/material/Divider';
import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants';
import { usePrpcQuery } from '@/common/hooks/prpc_query';
import { ResultDb } from '@/common/services/resultdb';

import { TestVariantContextProvider } from './context';
import { ResultDetails } from './result_details';
import { ResultHeader } from './result_header';
import { ResultInfo } from './result_info';
import { ResultLogs } from './result_logs';
import { TestIdentifier } from './test_identifier';

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
  const variant = results.testVariants[0];
  const sources = variant.sourcesId ? results.sources[variant.sourcesId] : {};

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
      <TestVariantContextProvider
        invocationID={invID}
        variant={variant}
        project={project}
        sources={sources}
      >
        <TestIdentifier />
        <ResultInfo />
        <ResultHeader />
        <ResultDetails />
        <Divider orientation="horizontal" flexItem />
        <ResultLogs />
      </TestVariantContextProvider>
    </Grid>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="test-verdict">
    <TestVerdictPage />
  </RecoverableErrorBoundary>
);
