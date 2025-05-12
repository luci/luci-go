// Copyright 2024 The LUCI Authors.
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

import { useQuery } from '@tanstack/react-query';

import { AssociatedBugsBadge } from '@/analysis/components/associated_bugs_badge';
import { OutputClusterResponse } from '@/analysis/types';
import { useBatchedClustersClient } from '@/common/hooks/prpc_clients';
import { logging } from '@/common/tools/logging';
import { ClusterRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  TestResult,
  TestStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

export interface ResultAssociatedBugsBadgeProps {
  readonly project: string;
  readonly testResult: TestResult;
  /**
   * A fallback test ID when `testResult.testId === ''`.
   *
   * When a test result is included in a test verdict, some common properties
   * (e.g. test_id, variant, variant_hash, source_metadata) are striped by the
   * RPC to reduce the size of the response payload. This property provides an
   * alternative way to specify the test ID.
   */
  readonly testId?: string;
}

export function ResultAssociatedBugsBadge({
  project,
  testResult,
  testId,
}: ResultAssociatedBugsBadgeProps) {
  const client = useBatchedClustersClient();
  const { data } = useQuery({
    ...client.Cluster.query(
      ClusterRequest.fromPartial({
        project,
        testResults: [
          {
            testId: testResult.testId || testId,
            failureReason: testResult.failureReason,
          },
        ],
      }),
    ),
    // The parent test verdict entry might've queried cluster for all test
    // results already.
    // Set the stale time to 10 mins so we don't trigger a 2nd request
    // immediately.
    staleTime: 600_000,
    enabled:
      !testResult.expected &&
      ![TestStatus.PASS, TestStatus.SKIP].includes(testResult.status),
    onError: (err) => {
      logging.error(err);
    },
    select: (res) => res as OutputClusterResponse,
  });

  if (!data) {
    return <></>;
  }

  return (
    <AssociatedBugsBadge
      project={project}
      clusters={data.clusteredTestResults[0].clusters}
    />
  );
}
