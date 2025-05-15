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

import { useQueries, useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { OutputClusterResponse } from '@/analysis/types';
import {
  useBatchedClustersClient,
  useTestVariantBranchesClient,
} from '@/common/hooks/prpc_clients';
import { AssociatedBug } from '@/common/services/luci_analysis';
import {
  ClusterRequest,
  ClusterResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  SourceRef as AnalysisSourceRef,
  GitilesRef as AnalysisGitilesRef,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { QueryTestVariantBranchRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { TestStatus as ResultDbTestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import {
  analyzeSegments,
  formatAllCLs,
} from '@/test_investigation/utils/test_info_utils';

import { TestInfoContext } from './context';
interface Props {
  children: React.ReactNode;
}

export function TestInfoProvider({ children }: Props) {
  const invocation = useInvocation();
  const testVariant = useTestVariant();
  const project = useProject();
  const allFormattedCLs = useMemo(
    () => formatAllCLs(invocation.sourceSpec?.sources?.changelists),
    [invocation.sourceSpec?.sources?.changelists],
  );
  const analysisClustersClient = useBatchedClustersClient();
  const analysisBranchesClient = useTestVariantBranchesClient();

  const resultsToCluster = useMemo(() => {
    if (!testVariant.results) return [];
    return testVariant.results
      .map((rlink) => rlink.result)
      .filter(
        (result) =>
          result &&
          !result.expected &&
          result.status &&
          ![ResultDbTestStatus.PASS, ResultDbTestStatus.SKIP].includes(
            result.status,
          ),
      );
  }, [testVariant]);

  const associatedBugsQueries = useQueries({
    queries: resultsToCluster.map((r) => ({
      ...analysisClustersClient.Cluster.query(
        ClusterRequest.fromPartial({
          project,
          testResults: [
            {
              testId: testVariant.testId,
              failureReason: r!.failureReason,
            },
          ],
        }),
      ),
      select: (res: ClusterResponse): OutputClusterResponse =>
        res as OutputClusterResponse,
      staleTime: 5 * 60 * 1000,
    })),
  });

  const isLoadingAssociatedBugs = associatedBugsQueries.some(
    (q) => q.isLoading,
  );

  const associatedBugs: AssociatedBug[] = useMemo(() => {
    // TODO: handle bug loading errors
    if (associatedBugsQueries.some((q) => !q.data || q.isError)) {
      return [];
    }
    const bugs = associatedBugsQueries
      .flatMap((q) =>
        q.data!.clusteredTestResults.flatMap((ctr) =>
          ctr.clusters.map((c) => c.bug),
        ),
      )
      .filter((bug) => bug !== undefined && bug !== null) as AssociatedBug[];

    const uniqueBugs = new Map<string, AssociatedBug>();
    bugs.forEach((bug) => {
      const key = `${bug.system}-${bug.id}`;
      if (!uniqueBugs.has(key)) {
        uniqueBugs.set(key, bug);
      }
    });
    return Array.from(uniqueBugs.values());
  }, [associatedBugsQueries]);

  const sourceRefForAnalysis: AnalysisSourceRef | undefined = useMemo(() => {
    const gc = invocation?.sourceSpec?.sources?.gitilesCommit;
    if (gc?.host && gc.project && gc.ref) {
      return AnalysisSourceRef.fromPartial({
        gitiles: AnalysisGitilesRef.fromPartial({
          host: gc.host,
          project: gc.project,
          ref: gc.ref,
        }),
      });
    }
    return undefined;
  }, [invocation?.sourceSpec?.sources?.gitilesCommit]);

  const testVariantBranchQueryEnabled = !!(
    project &&
    testVariant?.testId &&
    sourceRefForAnalysis &&
    testVariant?.variantHash
  );

  const testVariantBranchRequest = useMemo(() => {
    if (!testVariantBranchQueryEnabled)
      return QueryTestVariantBranchRequest.fromPartial({});
    return QueryTestVariantBranchRequest.fromPartial({
      project: project,
      testId: testVariant.testId,
      ref: sourceRefForAnalysis!,
    });
  }, [
    testVariantBranchQueryEnabled,
    project,
    testVariant,
    sourceRefForAnalysis,
  ]);

  const { data: testVariantBranch } = useQuery({
    ...analysisBranchesClient.Query.query(testVariantBranchRequest),
    enabled: testVariantBranchQueryEnabled,
    staleTime: 5 * 60 * 1000,
    select: (response) => {
      return (
        response.testVariantBranch?.find(
          (tvb) => tvb.variantHash === testVariant.variantHash,
        ) || null
      );
    },
  });

  const segmentAnalysis = useMemo(
    () => analyzeSegments(testVariantBranch?.segments),
    [testVariantBranch],
  );

  return (
    <TestInfoContext.Provider
      value={{
        associatedBugs,
        isLoadingAssociatedBugs,
        formattedCls: allFormattedCLs,
        segmentAnalysis,
        testVariantBranch,
      }}
    >
      {children}
    </TestInfoContext.Provider>
  );
}
