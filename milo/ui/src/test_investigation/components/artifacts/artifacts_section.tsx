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

import { Box, CircularProgress, Typography } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { parseTestResultName } from '@/common/tools/test_result_utils/index';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useTestVariant } from '@/test_investigation/context';
import { useIsLegacyInvocation } from '@/test_investigation/context/context';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';

import { ArtifactContentView, ArtifactSummaryView } from './artifact_content';
import { ArtifactTreeView } from './artifact_tree';
import { ArtifactsProvider, useArtifactsContext } from './context';
import { ArtifactTreeNodeData } from './types';

function ArtifactsSectionContent() {
  const resultDbClient = useResultDbClient();
  const { currentResult, selectedAttemptIndex } = useArtifactsContext();
  const isLegacyInvocation = useIsLegacyInvocation();

  const [selectedArtifactNode, setSelectedArtifactNode] =
    useState<ArtifactTreeNodeData | null>(null);

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
    refetchInterval: 10 * 60 * 1000,
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
        parent:
          isLegacyInvocation && currentResult?.name
            ? 'invocations/' +
              parseTestResultName(currentResult.name).invocationId
            : undefined,
        pageSize: 1000,
      }),
    ),
    enabled: !!currentResult?.name && isLegacyInvocation,
    staleTime: Infinity,
    refetchInterval: 10 * 60 * 1000,
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
      (art) => art.artifactId === 'text_diff',
    );
  }, [testResultArtifactsData]);

  const handleArtifactNodeSelect = (node: ArtifactTreeNodeData | null) => {
    setSelectedArtifactNode(node);
  };

  const isOverallArtifactListsLoading =
    isLoadingTestResultArtifacts ||
    (isLegacyInvocation && isLoadingInvocationScopeArtifacts);

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
      ) : currentResult ? (
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
                selectedArtifact={selectedArtifactNode}
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
                    selectedAttemptIndex={selectedAttemptIndex}
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
        <Typography color="text.disabled">
          {isOverallArtifactListsLoading ? (
            'Loading...'
          ) : !currentResult ? (
            'No test results to display.'
          ) : (
            <Box sx={{ pl: 1 }}>
              <ArtifactSummaryView
                currentResult={currentResult}
                textDiffArtifact={textDiffArtifact}
                selectedAttemptIndex={selectedAttemptIndex}
              />
            </Box>
          )}
        </Typography>
      )}
    </Box>
  );
}

export function ArtifactsSection() {
  const testVariant = useTestVariant();

  if (!testVariant.results) {
    return (
      <Typography color="text.disabled">No test results to display.</Typography>
    );
  }

  return (
    <ArtifactsProvider results={testVariant.results}>
      <ArtifactsSectionContent />
    </ArtifactsProvider>
  );
}
