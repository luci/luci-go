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

import { GrpcError } from '@chopsui/prpc-client';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Divider from '@mui/material/Divider';

import { useProject } from '@/common/components/page_meta/page_meta_provider';
import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { ClustersService } from '@/common/services/luci_analysis';
import { TestResultBundle, TestStatus } from '@/common/services/resultdb';

import { useTestVerdict } from '../context';

import { TestResultsProvider } from './context';
import { ResultDetails } from './result_details';
import { ResultLogs } from './result_logs';
import { ResultsHeader } from './results_header';

interface Props {
  readonly results: readonly TestResultBundle[];
}

export function TestResults({ results }: Props) {
  const project = useProject();
  const verdict = useTestVerdict();
  // We filter out skipped, passed, or expected results as these are not clustered.
  const filteredResults = results.filter(
    (r) =>
      !r.result.expected &&
      ![TestStatus.Pass, TestStatus.Skip].includes(r.result.status),
  );
  const {
    data: clustersResponse,
    error,
    isError,
  } = usePrpcQuery({
    Service: ClustersService,
    host: SETTINGS.luciAnalysis.host,
    method: 'cluster',
    options: {
      enabled: !!project,
    },
    request: {
      // The request is only enabled if the project is set.
      project: project!,
      testResults: filteredResults.map((r) => ({
        testId: verdict.testId,
        failureReason: r.result.failureReason && {
          primaryErrorMessage: r.result.failureReason.primaryErrorMessage,
        },
      })),
    },
  });

  if (error && !(error instanceof GrpcError)) {
    throw error;
  }

  const resultsClustersMap = new Map(
    clustersResponse?.clusteredTestResults.map((ctr, i) => [
      filteredResults[i].result.resultId,
      ctr.clusters,
    ]),
  );

  return (
    <TestResultsProvider results={results} clustersMap={resultsClustersMap}>
      {isError && (
        <Alert severity="error">
          <AlertTitle>Failed to load clusters for results</AlertTitle>
          Loading clusters failed due to:{' '}
          {error instanceof GrpcError && error.message}
        </Alert>
      )}
      <ResultsHeader />
      <ResultDetails />
      <Divider orientation="horizontal" flexItem />
      <ResultLogs />
    </TestResultsProvider>
  );
}
