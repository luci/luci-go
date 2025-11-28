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
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useIsLegacyInvocation } from '@/test_investigation/context';

import { useArtifactsContext } from '../../context';
import { ArtifactTreeNodeData, SelectedArtifactSource } from '../../types';
import { ArtifactTreeNode } from '../artifact_tree_node';
import { useArtifactFilters } from '../context/context';
import { filterArtifacts } from '../util/tree_util';

function addArtifactsToTree(
  artifacts: readonly Artifact[],
  root: ArtifactTreeNodeData,
  idCounter: number,
  source: SelectedArtifactSource,
) {
  for (const artifact of artifacts) {
    const path = artifact.artifactId;
    const parts = path.split('/');
    let currentNode = root;

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      if (part) {
        let childNode: ArtifactTreeNodeData | undefined =
          currentNode.children.find((c) => c.name === part);

        if (!childNode) {
          childNode = { id: `${idCounter++}`, name: part, children: [] };
          currentNode.children.push(childNode);
        }
        currentNode = childNode as ArtifactTreeNodeData;
      }
    }
    currentNode.viewingSupported = artifact.hasLines;
    currentNode.size = Number(artifact.sizeBytes);
    currentNode.url = getRawArtifactURLPath(artifact.name);
    currentNode.artifact = artifact;
    currentNode.source = source;
    currentNode.id = artifact.artifactId;
  }
  return idCounter;
}

function buildArtifactsTree(
  resultArtifacts: readonly Artifact[],
  invocationArtifacts: readonly Artifact[],
): ArtifactTreeNodeData[] {
  const result: ArtifactTreeNodeData[] = [];

  result.push({
    id: 'summary_node',
    name: 'Summary',
    isSummary: true,
    children: [],
  });
  let lastInsertedId = 0;

  if (resultArtifacts.length > 0) {
    const resultArtifactsRoot: ArtifactTreeNodeData = {
      id: `${++lastInsertedId}`,
      name: 'Result artifacts',
      children: [],
    };
    lastInsertedId = addArtifactsToTree(
      resultArtifacts,
      resultArtifactsRoot,
      ++lastInsertedId,
      'result',
    );

    result.push(resultArtifactsRoot);
  }

  if (invocationArtifacts.length > 0) {
    const invocationArtifactsRoot: ArtifactTreeNodeData = {
      id: `${++lastInsertedId}`,
      name: 'Invocation artifacts',
      children: [],
    };
    addArtifactsToTree(
      invocationArtifacts,
      invocationArtifactsRoot,
      ++lastInsertedId,
      'invocation',
    );
    result.push(invocationArtifactsRoot);
  }
  return result;
}

interface ArtifactTreeViewInternalProps {
  resultArtifacts: readonly Artifact[];
  invArtifacts: readonly Artifact[];
}

function ArtifactTreeViewInternal({
  resultArtifacts,
  invArtifacts,
}: ArtifactTreeViewInternalProps) {
  const {
    debouncedSearchTerm,
    artifactTypes,
    hideEmptyFolders,
    setAvailableArtifactTypes,
  } = useArtifactFilters();
  const {
    selectedArtifact: selectedArtifactNode,
    setSelectedArtifact: updateSelectedArtifact,
  } = useArtifactsContext();

  // Sync available artifact types to context
  useEffect(() => {
    const allArtifacts = [...resultArtifacts, ...invArtifacts];
    const types = new Set<string>();
    for (const artifact of allArtifacts) {
      if (artifact.artifactType) {
        types.add(artifact.artifactType);
      }
    }
    const sortedTypes = Array.from(types).sort();
    setAvailableArtifactTypes(sortedTypes);

    return () => {
      setAvailableArtifactTypes([]);
    };
  }, [resultArtifacts, invArtifacts, setAvailableArtifactTypes]);

  const filteredResultArtifacts = useMemo(
    () =>
      filterArtifacts(resultArtifacts, { debouncedSearchTerm, artifactTypes }),
    [resultArtifacts, debouncedSearchTerm, artifactTypes],
  );

  const filteredInvArtifacts = useMemo(
    () => filterArtifacts(invArtifacts, { debouncedSearchTerm, artifactTypes }),
    [invArtifacts, debouncedSearchTerm, artifactTypes],
  );

  const initialArtifactsTree = useMemo(() => {
    return buildArtifactsTree(filteredResultArtifacts, filteredInvArtifacts);
  }, [filteredResultArtifacts, filteredInvArtifacts]);

  const finalArtifactsTree = useMemo(() => {
    if (!hideEmptyFolders) {
      return initialArtifactsTree;
    }

    function pruneEmptyFolders(
      nodes: ArtifactTreeNodeData[],
    ): ArtifactTreeNodeData[] {
      const prunedNodes: ArtifactTreeNodeData[] = [];
      for (const node of nodes) {
        const isLeaf = !!node.artifact || node.isSummary;
        if (isLeaf) {
          prunedNodes.push(node);
          continue;
        }

        const prunedChildren = pruneEmptyFolders(node.children);

        if (prunedChildren.length > 0) {
          prunedNodes.push({ ...node, children: prunedChildren });
        }
      }
      return prunedNodes;
    }

    return pruneEmptyFolders(initialArtifactsTree);
  }, [initialArtifactsTree, hideEmptyFolders]);

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
    <ArtifactTreeViewInternal
      resultArtifacts={testResultArtifactsData || []}
      invArtifacts={invocationScopeArtifactsData || []}
    />
  );
}
