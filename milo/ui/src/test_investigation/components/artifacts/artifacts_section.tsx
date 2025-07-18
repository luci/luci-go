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

import {
  Box,
  CircularProgress,
  SelectChangeEvent,
  Typography,
} from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { parseTestResultName } from '@/common/tools/test_result_utils/index';
import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  SkippedReason_Kind,
  skippedReason_KindToJSON,
  TestResult,
  TestResult_Status,
  testResult_StatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useTestVariant } from '@/test_investigation/context';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';
import { normalizeFailureReason } from '@/test_investigation/utils/test_variant_utils';

import { ArtifactContentView, ArtifactSummaryView } from './artifact_content';
import { ArtifactTreeView } from './artifact_tree';
import { ClusteredResult, ArtifactTreeNodeData } from './types';

interface ClusterGroupData {
  results: TestResultBundle[];
  originalFailureReason: string;
  normalizedReasonKeyPart: string;
  failureKindKeyPart: FailureReason_Kind;
  skippedKindKeyPart: SkippedReason_Kind;
  statusV2KeyPart: TestResult_Status;
}

function getClusterSortPriority(status: TestResult_Status): number {
  switch (status) {
    case TestResult_Status.FAILED:
      return 1;
    case TestResult_Status.PASSED:
      return 2;
    case TestResult_Status.SKIPPED:
      return 3;
    case TestResult_Status.EXECUTION_ERRORED:
      return 4;
    case TestResult_Status.PRECLUDED:
      return 5;
    case TestResult_Status.STATUS_UNSPECIFIED:
    default:
      // Place unspecified or any new/unexpected statuses last.
      return 6;
  }
}

function clusterAndSortResults(
  results: readonly TestResultBundle[],
): ClusteredResult[] {
  const clusters = new Map<string, ClusterGroupData>();

  results.forEach((bundle: TestResultBundle) => {
    const reasonMsg = bundle.result?.failureReason?.primaryErrorMessage;
    const failureKind =
      bundle.result?.failureReason?.kind || FailureReason_Kind.KIND_UNSPECIFIED;
    const skippedKind =
      bundle.result?.skippedReason?.kind || SkippedReason_Kind.KIND_UNSPECIFIED;
    const statusV2 =
      bundle.result?.statusV2 || TestResult_Status.STATUS_UNSPECIFIED;

    const normalizedReason = normalizeFailureReason(reasonMsg);
    const failureKindStr = failureReason_KindToJSON(failureKind);
    const skippedKindStr = skippedReason_KindToJSON(skippedKind);
    const statusV2Str = testResult_StatusToJSON(statusV2);

    const key = `${statusV2Str}|${failureKindStr}|${skippedKindStr}|${normalizedReason}`;

    if (!clusters.has(key)) {
      clusters.set(key, {
        results: [],
        originalFailureReason: reasonMsg || '',
        normalizedReasonKeyPart: normalizedReason,
        failureKindKeyPart: failureKind,
        skippedKindKeyPart: skippedKind,
        statusV2KeyPart: statusV2,
      });
    }
    clusters.get(key)!.results.push(bundle);
  });

  const newClusteredFailures: ClusteredResult[] = Array.from(
    clusters.entries(),
  ).map(([key, data]) => ({
    clusterKey: key,
    results: data.results,
    normalizedReasonKeyPart: data.normalizedReasonKeyPart,
    failureKindKeyPart: data.failureKindKeyPart,
    skippedKindKeyPart: data.skippedKindKeyPart,
    statusV2KeyPart: data.statusV2KeyPart,
    originalFailureReason:
      data.originalFailureReason ||
      (key === 'Unknown' || key === 'No failure reason string specified.'
        ? 'No failure reason string specified.'
        : key),
  }));

  // Sort clusters by statusV2 according to the desired order.
  newClusteredFailures.sort(
    (a, b) =>
      getClusterSortPriority(a.statusV2KeyPart) -
      getClusterSortPriority(b.statusV2KeyPart),
  );

  return newClusteredFailures;
}

export function ArtifactsSection() {
  const resultDbClient = useResultDbClient();
  const testVariant = useTestVariant();

  const [selectedArtifactNode, setSelectedArtifactNode] =
    useState<ArtifactTreeNodeData | null>(null);

  const [clusteredFailures, setClusteredFailures] = useState<ClusteredResult[]>(
    [],
  );
  const [selectedClusterIndex, setSelectedClusterIndex] = useState<number>(0);
  const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number>(0);

  useEffect(() => {
    const newClusteredFailures = clusterAndSortResults(testVariant.results);
    setClusteredFailures(newClusteredFailures);
    setSelectedClusterIndex(0);
    setSelectedAttemptIndex(0);
  }, [testVariant.results]);

  const currentCluster: ClusteredResult | undefined =
    clusteredFailures[selectedClusterIndex];
  const currentAttempts: TestResultBundle[] = currentCluster?.results || [];
  const currentAttemptBundle: TestResultBundle | undefined =
    currentAttempts[selectedAttemptIndex];
  const currentResult: TestResult | undefined = currentAttemptBundle?.result;

  const {
    data: testResultArtifactsData,
    isPending: isLoadingTestResultArtifacts,
    hasNextPage: testResultArtifactsHasNextPage,
    fetchNextPage: loadMoreTestResultArtifacts,
  } = useInfiniteQuery({
    ...resultDbClient.ListArtifacts.queryPaged(
      ListArtifactsRequest.fromPartial({
        parent: currentResult?.name,
        pageSize: 1000,
      }),
    ),
    enabled: !!currentResult?.name,
    staleTime: Infinity,
    refetchInterval: 10 * 60 * 1000, // Refetch every 10 minutes to refresh the signed links.
    select: (res) => res.pages.flatMap((page) => page.artifacts) || [],
  });

  useEffect(() => {
    if (!isLoadingTestResultArtifacts && testResultArtifactsHasNextPage) {
      loadMoreTestResultArtifacts();
    }
  }, [
    isLoadingTestResultArtifacts,
    loadMoreTestResultArtifacts,
    testResultArtifactsHasNextPage,
  ]);

  const {
    data: invocationScopeArtifactsData,
    isPending: isLoadingInvocationScopeArtifacts,
    hasNextPage: invocationScopeArtifactsHasNextPage,
    fetchNextPage: loadMoreInvocationScopeArtifacts,
  } = useInfiniteQuery({
    ...resultDbClient.ListArtifacts.queryPaged(
      ListArtifactsRequest.fromPartial({
        parent: currentResult?.name
          ? 'invocations/' +
            parseTestResultName(currentResult.name).invocationId
          : undefined,
        pageSize: 1000,
      }),
    ),
    enabled: !!currentResult?.name,
    staleTime: Infinity,
    refetchInterval: 10 * 60 * 1000, // Refetch every 10 minutes to refresh the signed links.
    select: (res) => res.pages.flatMap((page) => page.artifacts) || [],
  });

  useEffect(() => {
    if (
      !isLoadingInvocationScopeArtifacts &&
      invocationScopeArtifactsHasNextPage
    ) {
      loadMoreInvocationScopeArtifacts();
    }
  }, [
    isLoadingInvocationScopeArtifacts,
    loadMoreInvocationScopeArtifacts,
    invocationScopeArtifactsHasNextPage,
  ]);

  const artifactContentQueryEnabled =
    !!selectedArtifactNode?.artifact?.fetchUrl &&
    !selectedArtifactNode.isSummary;

  const { data: artifactContentData, isPending: rawIsLoadingArtifactContent } =
    useFetchArtifactContentQuery({
      artifactContentQueryEnabled,
      isSummary: selectedArtifactNode?.isSummary,
      artifact: selectedArtifactNode?.artifact,
    });

  const isLoadingArtifactContent =
    artifactContentQueryEnabled && rawIsLoadingArtifactContent;

  const textDiffArtifact = useMemo(() => {
    return (testResultArtifactsData || []).find(
      (art) => art.artifactId === 'text_diff', // Updated artifactId
    );
  }, [testResultArtifactsData]);

  const handleArtifactNodeSelect = (node: ArtifactTreeNodeData | null) => {
    setSelectedArtifactNode(node);
  };

  const isOverallArtifactListsLoading =
    isLoadingTestResultArtifacts || isLoadingInvocationScopeArtifacts;

  const containsArtifacts = useMemo(() => {
    return (
      (testResultArtifactsData && testResultArtifactsData.length > 0) ||
      (invocationScopeArtifactsData && invocationScopeArtifactsData?.length > 0)
    );
  }, [testResultArtifactsData, invocationScopeArtifactsData]);

  const handleClusterChange = (event: SelectChangeEvent<number>) => {
    setSelectedClusterIndex(Number(event.target.value));
    setSelectedAttemptIndex(0);
  };

  const handleAttemptChange = (event: SelectChangeEvent<number>) => {
    setSelectedAttemptIndex(Number(event.target.value));
  };

  const hasRenderableResults =
    testVariant.results && testVariant.results.length > 0;

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {isOverallArtifactListsLoading ? (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: '380px',
          }}
        >
          <CircularProgress />
          <Typography sx={{ ml: 1 }}>Loading artifact lists...</Typography>
        </Box>
      ) : currentResult && containsArtifacts ? (
        <PanelGroup
          direction="horizontal"
          style={{ height: '100%', minHeight: '600px' }}
        >
          <Panel defaultSize={30} minSize={20}>
            <Box
              sx={{
                height: '100%',
                overflowY: 'auto',
                borderRight: '1px solid',
                borderColor: 'divider',
              }}
            >
              <ArtifactTreeView
                artifactsLoading={isOverallArtifactListsLoading}
                invArtifacts={invocationScopeArtifactsData || []}
                resultArtifacts={testResultArtifactsData || []}
                currentResult={currentResult}
                selectedArtifact={selectedArtifactNode}
                clusteredFailures={clusteredFailures}
                selectedClusterIndex={selectedClusterIndex}
                onClusterChange={handleClusterChange}
                currentAttempts={currentAttempts}
                selectedAttemptIndex={selectedAttemptIndex}
                onAttemptChange={handleAttemptChange}
                currentCluster={currentCluster}
                hasRenderableResults={hasRenderableResults}
                updateSelectedArtifact={handleArtifactNodeSelect}
              />
            </Box>
          </Panel>
          <PanelResizeHandle>
            <Box
              sx={{
                width: '8px',
                height: '100%',
                cursor: 'col-resize',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
              }}
            >
              <Box sx={{ width: '2px', height: '24px', bgcolor: 'divider' }} />
            </Box>
          </PanelResizeHandle>
          <Panel defaultSize={70} minSize={30}>
            <Box
              sx={{
                p: 2,
                height: '100%',
                overflowY: 'auto',
                wordBreak: 'break-all',
              }}
            >
              {selectedArtifactNode &&
                (selectedArtifactNode.isSummary && currentResult ? (
                  <ArtifactSummaryView
                    currentResult={currentResult}
                    textDiffArtifact={textDiffArtifact}
                  />
                ) : selectedArtifactNode.artifact ? (
                  <ArtifactContentView
                    selectedArtifactForDisplay={selectedArtifactNode}
                    currentResult={currentResult}
                    artifactContentData={artifactContentData}
                    isLoadingArtifactContent={isLoadingArtifactContent}
                    invocationHasArtifacts={
                      (invocationScopeArtifactsData &&
                        invocationScopeArtifactsData.length > 0) ||
                      false
                    }
                  />
                ) : (
                  <Typography color="text.secondary">
                    Select an artifact to view.
                  </Typography>
                ))}
            </Box>
          </Panel>
        </PanelGroup>
      ) : (
        <Typography sx={{ p: 2 }} color="text.disabled">
          {isOverallArtifactListsLoading
            ? 'Loading...'
            : !currentResult
              ? 'No test results to display.'
              : 'No summary or artifacts available.'}
        </Typography>
      )}
    </Box>
  );
}
