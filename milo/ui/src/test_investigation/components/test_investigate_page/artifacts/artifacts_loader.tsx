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

import { CircularProgress, Box, Typography } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { InfiniteData } from '@tanstack/react-query';
import { ReactNode, useEffect, useMemo } from 'react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { parseTestResultName } from '@/common/tools/test_result_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  ListArtifactsRequest,
  ListArtifactsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { ArtifactsProvider } from '@/test_investigation/components/common/artifacts/context';
import { useArtifactFilters } from '@/test_investigation/components/common/artifacts/tree/context/context';
import {
  buildArtifactsTree,
  filterArtifacts,
  findFirstLeafRecursive,
  findNode,
  pruneEmptyFolders,
} from '@/test_investigation/components/common/artifacts/tree/util/tree_util';
import {
  useInvocation,
  useIsLegacyInvocation,
} from '@/test_investigation/context';

import { useArtifactsContext } from './context';

interface ArtifactsLoaderProps {
  children: ReactNode;
}

const EMPTY_ARRAY: readonly Artifact[] = [];

const selectArtifacts = (res: InfiniteData<ListArtifactsResponse, string>) =>
  res.pages.flatMap((page) => page.artifacts) || EMPTY_ARRAY;

export function ArtifactsLoader({ children }: ArtifactsLoaderProps) {
  const resultDbClient = useResultDbClient();
  const { currentResult } = useArtifactsContext();
  const isLegacyInvocation = useIsLegacyInvocation();
  const invocation = useInvocation();

  const [searchParams] = useSyncedSearchParams();
  const selectedArtifactId = searchParams.get('artifact');

  const {
    debouncedSearchTerm,
    artifactTypes,
    hideEmptyFolders,
    setAvailableArtifactTypes,
  } = useArtifactFilters();

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
    select: selectArtifacts,
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
    select: selectArtifacts,
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

  const resultArtifacts = testResultArtifactsData || EMPTY_ARRAY;
  const invArtifacts = invocationScopeArtifactsData || EMPTY_ARRAY;

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
      filterArtifacts(resultArtifacts, {
        searchTerm: debouncedSearchTerm,
        artifactTypes,
      }),
    [resultArtifacts, debouncedSearchTerm, artifactTypes],
  );

  const filteredInvArtifacts = useMemo(
    () =>
      filterArtifacts(invArtifacts, {
        searchTerm: debouncedSearchTerm,
        artifactTypes,
      }),
    [invArtifacts, debouncedSearchTerm, artifactTypes],
  );

  const initialArtifactsTree = useMemo(() => {
    return buildArtifactsTree(filteredResultArtifacts, filteredInvArtifacts);
  }, [filteredResultArtifacts, filteredInvArtifacts]);

  const finalArtifactsTree = useMemo(() => {
    if (!hideEmptyFolders) {
      return initialArtifactsTree;
    }
    return pruneEmptyFolders(initialArtifactsTree);
  }, [initialArtifactsTree, hideEmptyFolders]);

  const { selectedArtifact, setSelectedArtifact } = useArtifactsContext();

  useEffect(() => {
    if (debouncedSearchTerm) return;
    if (selectedArtifact) return;

    if (selectedArtifactId) {
      const node = findNode(finalArtifactsTree, selectedArtifactId);
      if (node) {
        setSelectedArtifact(node);
      }
      return;
    }

    const summaryNode = finalArtifactsTree.find((node) => node.isSummary);
    if (summaryNode) {
      setSelectedArtifact(summaryNode);
      return;
    }
    const firstLeaf = findFirstLeafRecursive(finalArtifactsTree);
    if (firstLeaf) {
      setSelectedArtifact(firstLeaf);
    }
  }, [
    finalArtifactsTree,
    selectedArtifact,
    setSelectedArtifact,
    debouncedSearchTerm,
    selectedArtifactId,
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
    <ArtifactsProvider
      nodes={finalArtifactsTree}
      selectedNodeId={selectedArtifact?.id}
      onSelect={setSelectedArtifact}
      invocation={invocation}
    >
      {children}
    </ArtifactsProvider>
  );
}
