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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  CircularProgress,
  Typography,
} from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';
import { parseTestResultName } from '@/test_verdict/tools/utils';

import { ArtifactContentView } from './artifact_content_view';
import { ArtifactTreeView } from './artifact_tree_view';
import { ArtifactTreeNodeData } from './types';

interface ArtifactsSectionProps {
  currentResult: TestResult;
  invocationName: string;
  panelId: string;
  headerId: string;
}

export function ArtifactsSection({
  currentResult,
  invocationName,
  panelId,
  headerId,
}: ArtifactsSectionProps) {
  const resultDbClient = useResultDbClient();

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
          'invocations/' +
          parseTestResultName(currentResult?.name).invocationId,
        pageSize: 1000,
      }),
    ),
    enabled: !!invocationName,
    staleTime: Infinity,
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

  return (
    <Accordion defaultExpanded>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        aria-controls={panelId}
        id={headerId}
      >
        <Typography
          sx={{
            fontWeight: 500,
            fontSize: '1rem',
            color: 'var(--gm3-color-on-surface-strong)',
          }}
        >
          Artifacts
        </Typography>
      </AccordionSummary>
      <AccordionDetails id={panelId} sx={{ p: 0, minHeight: '400px' }}>
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
          ) : containsArtifacts ? (
            <PanelGroup
              direction="horizontal"
              style={{ height: '100%', minHeight: '380px' }}
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
                  <Box
                    sx={{ width: '2px', height: '24px', bgcolor: 'divider' }}
                  />
                </Box>
              </PanelResizeHandle>
              <Panel defaultSize={70} minSize={30}>
                <Box sx={{ p: 2, height: '100%', overflowY: 'auto' }}>
                  {selectedArtifactNode && (
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
                  )}
                </Box>
              </Panel>
            </PanelGroup>
          ) : (
            <Typography sx={{ p: 2 }} color="text.disabled">
              {isOverallArtifactListsLoading
                ? 'Loading artifact lists...'
                : 'No summary or artifacts available.'}
            </Typography>
          )}
        </Box>
      </AccordionDetails>
    </Accordion>
  );
}
