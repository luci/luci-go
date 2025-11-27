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
import { useCallback, useEffect, useMemo } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { parseTestResultName } from '@/common/tools/test_result_utils/index';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useIsLegacyInvocation } from '@/test_investigation/context';

import { useArtifactsContext } from '../context';
import { ArtifactTreeNodeData } from '../types';

import { ArtifactsTreeLayout } from './artifact_tree_layout';
import { ArtifactTreeNode } from './artifact_tree_node';
import { useArtifactFilters } from './context/context';
import { ArtifactFilterProvider } from './context/provider';

function ArtifactTreeViewInternal() {
  const { debouncedSearchTerm, finalArtifactsTree } = useArtifactFilters();
  const {
    selectedArtifact: selectedArtifactNode,
    setSelectedArtifact: updateSelectedArtifact,
  } = useArtifactsContext();

  useEffect(() => {
    if (debouncedSearchTerm) return;
    if (selectedArtifactNode) return;

    const summaryNode = finalArtifactsTree.find((node) => node.isSummary);
    if (summaryNode) {
      updateSelectedArtifact(summaryNode);
      return;
    }

    const findFirstLeafRecursive = (
      nodes: readonly ArtifactTreeNodeData[],
    ): ArtifactTreeNodeData | null => {
      for (const node of nodes) {
        if (!node.children || node.children.length === 0) {
          if (node.artifact || node.isSummary) return node;
        }
        if (node.children) {
          const found = findFirstLeafRecursive(node.children);
          if (found) return found;
        }
      }
      return null;
    };

    const firstLeaf = findFirstLeafRecursive(finalArtifactsTree);
    updateSelectedArtifact(firstLeaf);
  }, [
    finalArtifactsTree,
    updateSelectedArtifact,
    selectedArtifactNode,
    debouncedSearchTerm,
  ]);

  const setActiveSelectionFnForSelectedNode = useCallback(
    (nodeData: ArtifactTreeNodeData): boolean => {
      if (!selectedArtifactNode) return false;
      if (selectedArtifactNode.isSummary) return !!nodeData.isSummary;
      return (
        nodeData.artifact?.artifactId ===
        selectedArtifactNode.artifact?.artifactId
      );
    },
    [selectedArtifactNode],
  );

  const selectedNodes: Set<string> | undefined = useMemo(() => {
    if (selectedArtifactNode) {
      return new Set([selectedArtifactNode.id]);
    }
    return undefined;
  }, [selectedArtifactNode]);

  function handleLeafNodeClicked(node: ArtifactTreeNodeData) {
    if (node.artifact || node.isSummary) {
      updateSelectedArtifact(node);
    }
  }

  function handleUnsupportedLeafNodeClicked(nodeData: ArtifactTreeNodeData) {
    open(
      getRawArtifactURLPath(nodeData.artifact?.name || nodeData.url || ''),
      '_blank',
    );
  }

  return (
    <ArtifactsTreeLayout>
      <VirtualTree<ArtifactTreeNodeData>
        root={finalArtifactsTree}
        isTreeCollapsed={false}
        scrollToggle
        itemContent={(
          index: number,
          row: TreeData<ArtifactTreeNodeData>,
          context: VirtualTreeNodeActions<ArtifactTreeNodeData>,
        ) => (
          <ArtifactTreeNode
            index={index}
            row={row}
            context={context}
            onSupportedLeafClick={handleLeafNodeClicked}
            onUnsupportedLeafClick={handleUnsupportedLeafNodeClicked}
            highlightText={debouncedSearchTerm}
          />
        )}
        selectedNodes={selectedNodes}
        setActiveSelectionFn={setActiveSelectionFnForSelectedNode}
      />
    </ArtifactsTreeLayout>
  );
}

export function ArtifactTreeView() {
  const resultDbClient = useResultDbClient();
  const { currentResult } = useArtifactsContext();
  const isLegacyInvocation = useIsLegacyInvocation();

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

  const isOverallArtifactListsLoading =
    isLoadingTestResultArtifacts ||
    (isLegacyInvocation && isLoadingInvocationScopeArtifacts);

  if (isOverallArtifactListsLoading) {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          p: 2,
        }}
      >
        <CircularProgress size={24} />
        <Typography sx={{ ml: 1 }}>Loading artifact lists...</Typography>
      </Box>
    );
  }

  return (
    <ArtifactFilterProvider
      resultArtifacts={testResultArtifactsData || []}
      invArtifacts={invocationScopeArtifactsData || []}
    >
      <ArtifactTreeViewInternal />
    </ArtifactFilterProvider>
  );
}
