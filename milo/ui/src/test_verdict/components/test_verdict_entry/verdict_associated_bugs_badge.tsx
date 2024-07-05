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

import { useQueries } from '@tanstack/react-query';
import { memo } from 'react';

import { AssociatedBugsBadge } from '@/analysis/components/associated_bugs_badge';
import { useBatchedClustersClient } from '@/analysis/hooks/batched_clusters_client';
import { OutputClusterResponse } from '@/analysis/types';
import { logging } from '@/common/tools/logging';
import {
  ClusterRequest,
  ClusterResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { OutputTestVerdict } from '@/test_verdict/types';

export interface VerdictAssociatedBugsBadgeProps {
  readonly project: string;
  readonly verdict: OutputTestVerdict;
}

export const VerdictAssociatedBugsBadge = memo(
  function VerdictAssociatedBugsBadge({
    project,
    verdict,
  }: VerdictAssociatedBugsBadgeProps) {
    const client = useBatchedClustersClient();
    const resultsToCluster = verdict.results
      .map((r) => r.result)
      .filter(
        (r) =>
          !r.expected && ![TestStatus.PASS, TestStatus.SKIP].includes(r.status),
      );
    const queries = useQueries({
      // Call the Cluster RPC once for each test result so
      // 1. react-query can handle deduplication, and
      // 2. when individual result entries make the call for an individual test
      //    results, the cache can be reused.
      //
      // This will not cause many RPCs to be fired in parallel because we uses
      // the batched clusters client. All RPC calls in the same rendering cycle
      // are batched together.
      queries: resultsToCluster.map((r) => ({
        ...client.Cluster.query(
          ClusterRequest.fromPartial({
            project,
            testResults: [
              {
                testId: verdict.testId,
                failureReason: r.failureReason,
              },
            ],
          }),
        ),
        select: (res: ClusterResponse) => res as OutputClusterResponse,
        onError: (err: unknown) => logging.error(err),
      })),
    });
    if (queries.some((q) => !q.data)) {
      return <></>;
    }

    const clusters = queries.flatMap((q) =>
      q.data!.clusteredTestResults.flatMap((c) => c.clusters),
    );

    return <AssociatedBugsBadge project={project} clusters={clusters} />;
  },
);
