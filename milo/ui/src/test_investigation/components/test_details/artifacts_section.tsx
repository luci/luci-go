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
import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState, JSX } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';

import {
  buildCustomArtifactTreeNodes,
  INVOCATION_ARTIFACTS_ROOT_ID,
  RESULT_ARTIFACTS_ROOT_ID,
  SUMMARY_NODE_ID_TOP_LEVEL,
} from '../../utils/artifact_utils';

import { ArtifactContentView } from './artifact_content_view';
import { ArtifactTreeView } from './artifact_tree_view';
import { CustomArtifactTreeNode } from './types';

interface ArtifactsSectionProps {
  currentResult?: TestResult;
  invocationName: string;
  panelId: string;
  headerId: string;
}

export function ArtifactsSection({
  currentResult,
  invocationName,
  panelId,
  headerId,
}: ArtifactsSectionProps): JSX.Element {
  const resultDbClient = useResultDbClient();

  const [selectedArtifactForDisplay, setSelectedArtifactForDisplay] =
    useState<CustomArtifactTreeNode | null>(null);
  const [expandedArtifactNodeIds, setExpandedArtifactNodeIds] = useState<
    Set<string>
  >(new Set([RESULT_ARTIFACTS_ROOT_ID, INVOCATION_ARTIFACTS_ROOT_ID]));

  const {
    data: testResultArtifactsData,
    isPending: isLoadingTestResultArtifacts,
  } = useQuery({
    ...resultDbClient.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: currentResult?.name,
        pageSize: 1000,
      }),
    ),
    enabled: !!currentResult?.name,
    staleTime: Infinity,
    select: (res) => res.artifacts || [],
  });

  const {
    data: invocationScopeArtifactsData,
    isPending: isLoadingInvocationScopeArtifacts,
  } = useQuery({
    ...resultDbClient.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: invocationName,
        pageSize: 1000,
      }),
    ),
    enabled: !!invocationName,
    staleTime: Infinity,
    select: (res) => res.artifacts || [],
  });

  const artifactTreeRootNodes = useMemo(
    () =>
      buildCustomArtifactTreeNodes(
        currentResult,
        testResultArtifactsData || undefined,
        invocationScopeArtifactsData || undefined,
      ),
    [currentResult, testResultArtifactsData, invocationScopeArtifactsData],
  );

  useEffect(() => {
    // This effect ensures that after currentResult (and thus artifacts) changes,
    // a sensible default selection is made in the artifact tree.
    const currentNodes = buildCustomArtifactTreeNodes(
      currentResult,
      testResultArtifactsData || undefined,
      invocationScopeArtifactsData || undefined,
    );
    const summaryNode = currentNodes.find(
      (node) => node.id === SUMMARY_NODE_ID_TOP_LEVEL,
    );

    if (summaryNode) {
      setSelectedArtifactForDisplay(summaryNode);
    } else if (currentNodes.length > 0) {
      const findFirstLeafRecursive = (
        nodes: CustomArtifactTreeNode[],
      ): CustomArtifactTreeNode | null => {
        for (const node of nodes) {
          if (node.isLeaf && !node.isSummary && node.artifact) return node;
          if (node.children) {
            const found = findFirstLeafRecursive(node.children);
            if (found) return found;
          }
        }
        for (const node of nodes) {
          // Fallback for top-level leaf if no nested found
          if (node.isLeaf && node.artifact) return node;
        }
        return null;
      };
      const firstLeaf = findFirstLeafRecursive(currentNodes);
      setSelectedArtifactForDisplay(firstLeaf);
    } else {
      setSelectedArtifactForDisplay(null);
    }

    // Ensure root folders are expanded by default if they exist
    setExpandedArtifactNodeIds((prev) => {
      const newSet = new Set(prev); // Keep existing user expansions
      if (currentNodes.some((n) => n.id === RESULT_ARTIFACTS_ROOT_ID))
        newSet.add(RESULT_ARTIFACTS_ROOT_ID);
      if (currentNodes.some((n) => n.id === INVOCATION_ARTIFACTS_ROOT_ID))
        newSet.add(INVOCATION_ARTIFACTS_ROOT_ID);
      return newSet;
    });
  }, [currentResult, testResultArtifactsData, invocationScopeArtifactsData]);

  const artifactContentQueryEnabled =
    !!selectedArtifactForDisplay?.artifact?.fetchUrl && // User fixed to fetchUrl
    !selectedArtifactForDisplay.isSummary;

  const { data: artifactContentData, isPending: rawIsLoadingArtifactContent } =
    useFetchArtifactContentQuery({
      artifactContentQueryEnabled,
      isSummary: selectedArtifactForDisplay?.isSummary,
      artifact: selectedArtifactForDisplay?.artifact,
    });

  const isLoadingArtifactContent =
    artifactContentQueryEnabled && rawIsLoadingArtifactContent;

  const handleArtifactNodeSelect = (node: CustomArtifactTreeNode) => {
    setSelectedArtifactForDisplay(node);
  };

  const handleArtifactNodeToggle = (nodeId: string) => {
    setExpandedArtifactNodeIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(nodeId)) newSet.delete(nodeId);
      else newSet.add(nodeId);
      return newSet;
    });
  };

  const isOverallArtifactListsLoading =
    isLoadingTestResultArtifacts || isLoadingInvocationScopeArtifacts;

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
          {isOverallArtifactListsLoading &&
          artifactTreeRootNodes.length === 0 ? (
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
          ) : artifactTreeRootNodes.length > 0 ? (
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
                  {artifactTreeRootNodes.map((node) => (
                    <ArtifactTreeView
                      key={node.id}
                      node={node}
                      selectedNodeId={selectedArtifactForDisplay?.id || null}
                      onNodeSelect={handleArtifactNodeSelect}
                      expandedNodeIds={expandedArtifactNodeIds}
                      onNodeToggle={handleArtifactNodeToggle}
                    />
                  ))}
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
                  <ArtifactContentView
                    selectedArtifactForDisplay={selectedArtifactForDisplay}
                    currentResult={currentResult}
                    artifactContentData={artifactContentData}
                    isLoadingArtifactContent={isLoadingArtifactContent}
                    invocationHasArtifacts={
                      (invocationScopeArtifactsData &&
                        invocationScopeArtifactsData.length > 0) ||
                      false
                    }
                  />
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
