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

import { GrpcError, ProtocolError } from '@chopsui/prpc-client';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import { useQuery } from '@tanstack/react-query';

import { useBatchedClustersClient } from '@/analysis/hooks/prpc_clients';
import { OutputClusterEntry } from '@/analysis/types';
import { ClusterRequest_TestResult } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { OutputTestResultBundle } from '@/test_verdict/types';

import { useProject, useTestVerdict } from '../context';

import { TestResultsProvider } from './context';
import { ResultDetails } from './result_details';
import { ResultsHeader } from './results_header';

interface Props {
  readonly results: readonly OutputTestResultBundle[];
}

export function TestResults({ results }: Props) {
  const project = useProject();
  const verdict = useTestVerdict();

  // We filter out skipped, passed, or expected results as these are not clustered.
  const filteredResults = results.filter(
    (r) =>
      !r.result.expected &&
      ![TestStatus.PASS, TestStatus.SKIP].includes(r.result.status),
  );

  const client = useBatchedClustersClient();
  const {
    data: clustersResponse,
    error,
    isError,
  } = useQuery({
    ...client.Cluster.query({
      // The request is only enabled if the project is set.
      project: project!,
      testResults: filteredResults.map((r) =>
        ClusterRequest_TestResult.fromPartial({
          testId: verdict.testId,
          failureReason: r.result.failureReason && {
            primaryErrorMessage: r.result.failureReason.primaryErrorMessage,
          },
        }),
      ),
    }),
    enabled: !!project,
  });

  const isReqError =
    error instanceof GrpcError || error instanceof ProtocolError;
  if (isError && !isReqError) {
    throw error;
  }

  const resultsClustersMap = new Map(
    clustersResponse?.clusteredTestResults.map((ctr, i) => [
      filteredResults[i].result.resultId,
      ctr.clusters as readonly OutputClusterEntry[],
    ]),
  );

  return (
    <TestResultsProvider results={results} clustersMap={resultsClustersMap}>
      {isReqError && (
        <Alert severity="error">
          <AlertTitle>Failed to load clusters for results</AlertTitle>
          Loading clusters failed due to: {error.message}
        </Alert>
      )}
      <ResultsHeader />
      <ResultDetails />
    </TestResultsProvider>
  );
}
