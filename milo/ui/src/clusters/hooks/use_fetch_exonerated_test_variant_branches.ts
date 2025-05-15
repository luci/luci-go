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

import {
  useClustersService,
  useTestVariantsService,
} from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import { ClusterExoneratedTestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { Variant } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  GitilesCommit,
  SourceRef,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import {
  QueryTestVariantStabilityRequest,
  QueryTestVariantStabilityRequest_TestVariantPosition,
  TestStabilityCriteria,
  TestVariantStabilityAnalysis,
  TestVariantStabilityAnalysis_FailureRate,
  TestVariantStabilityAnalysis_FlakeRate,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

const useFetchExoneratedTestVariantBranches = (
  project: string,
  clusterAlgorithm: string,
  clusterId: string,
) => {
  const clusterService = useClustersService();
  const tvService = useTestVariantsService();

  return useQuery({
    queryKey: [
      'exoneratedTestVariantBranches',
      project,
      clusterAlgorithm,
      clusterId,
    ],

    queryFn: async () => {
      const clusterResponse =
        await clusterService.QueryExoneratedTestVariantBranches({
          parent: `projects/${project}/clusters/${clusterAlgorithm}/${clusterId}/exoneratedTestVariantBranches`,
        });
      const clusterExoneratedTestVariantBranches =
        clusterResponse.testVariantBranches;
      if (clusterExoneratedTestVariantBranches.length === 0) {
        // The criteria is irrelevant as there are no branches to display.
        // Return a filler criteria.
        const emptyCriteria: TestStabilityCriteria = {
          failureRate: {
            failureThreshold: 0,
            consecutiveFailureThreshold: 0,
          },
          flakeRate: {
            flakeRateThreshold: 0.0,
            flakeThreshold: 0,
            minWindow: 0,
            flakeThreshold1wd: 0,
          },
        };
        const result: ExoneratedTestVariantBranches = {
          testVariantBranches: [],
          criteria: emptyCriteria,
        };
        return result;
      }
      const tvRequest: QueryTestVariantStabilityRequest = {
        project: project,
        testVariants: clusterExoneratedTestVariantBranches.map((tvb) => {
          let gitilesCommit: GitilesCommit | undefined;
          if (tvb.sourceRef?.gitiles) {
            gitilesCommit = {
              host: tvb.sourceRef.gitiles.host,
              ref: tvb.sourceRef.gitiles.ref,
              project: tvb.sourceRef.gitiles.project,
              // Query for the most recent commit position.
              commitHash: 'ff'.repeat(20),
              position: '999999999999',
            };
          }
          return QueryTestVariantStabilityRequest_TestVariantPosition.create({
            testId: tvb.testId,
            variant: tvb.variant,
            sources: {
              gitilesCommit: gitilesCommit,
            },
          });
        }),
      };
      const tvResponse = await tvService.QueryStability(tvRequest);

      const exoneratedTVBs =
        tvResponse.testVariants?.map((analyzedTV, i) => {
          // QueryFailureRate returns test variants in the same order
          // that they are requested.
          const exoneratedTV = clusterExoneratedTestVariantBranches[i];
          return testVariantBranchFromAnalysis(exoneratedTV, analyzedTV);
        }) || [];

      const result: ExoneratedTestVariantBranches = {
        testVariantBranches: exoneratedTVBs,
        // Criteria will always be set if the RPC succeeds.
        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        criteria: tvResponse.criteria!,
      };
      return result;
    },

    retry: prpcRetrier,
  });
};

export default useFetchExoneratedTestVariantBranches;

function testVariantBranchFromAnalysis(
  ctvb: ClusterExoneratedTestVariantBranch,
  analysis: TestVariantStabilityAnalysis,
): ExoneratedTestVariantBranch {
  return {
    testId: ctvb.testId,
    variant: ctvb.variant,
    // Source ref will always be set.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    sourceRef: ctvb.sourceRef!,
    // Last exoneration will always be set.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    lastExoneration: ctvb.lastExoneration!,
    criticalFailuresExonerated: ctvb.criticalFailuresExonerated,
    // It is a guarantee of the RPC that both failure rate and flake rate will be set.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    failureRate: analysis.failureRate!,
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    flakeRate: analysis.flakeRate!,
  };
}

export interface ExoneratedTestVariantBranches {
  testVariantBranches: ExoneratedTestVariantBranch[];
  criteria: TestStabilityCriteria;
}

export interface ExoneratedTestVariantBranch {
  testId: string;
  variant: Variant | undefined;
  sourceRef: SourceRef;
  // RFC 3339 encoded date/time of the last exoneration.
  lastExoneration: string;
  // The number of critical failures exonerated in the last week.
  criticalFailuresExonerated: number;

  failureRate: TestVariantStabilityAnalysis_FailureRate;
  flakeRate: TestVariantStabilityAnalysis_FlakeRate;
}
