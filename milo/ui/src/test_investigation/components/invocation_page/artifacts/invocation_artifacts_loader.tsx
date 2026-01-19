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
import { useInfiniteQuery, InfiniteData } from '@tanstack/react-query';
import { ReactNode, useCallback, useEffect, useMemo } from 'react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import {
  ListArtifactsRequest,
  ListArtifactsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { ArtifactsProvider } from '@/test_investigation/components/common/artifacts/context/provider';
import { useArtifactFilters } from '@/test_investigation/components/common/artifacts/tree/context/context';
import {
  buildArtifactsTree,
  filterArtifacts,
  findFirstLeafRecursive,
  pruneEmptyFolders,
} from '@/test_investigation/components/common/artifacts/tree/util/tree_util';
import { ArtifactTreeNodeData } from '@/test_investigation/components/common/artifacts/types';
import { useInvocation } from '@/test_investigation/context';
import {
  AnyInvocation,
  isRootInvocation,
} from '@/test_investigation/utils/invocation_utils';

interface InvocationArtifactsLoaderProps {
  children: ReactNode;
}

const EMPTY_ARRAY: readonly Artifact[] = [];

const selectArtifacts = (res: InfiniteData<ListArtifactsResponse, string>) =>
  res.pages.flatMap((page) => page.artifacts) || EMPTY_ARRAY;

export function InvocationArtifactsLoader({
  children,
}: InvocationArtifactsLoaderProps) {
  const resultDbClient = useResultDbClient();
  const invocation = useInvocation();

  const {
    debouncedSearchTerm,
    artifactTypes,
    hideEmptyFolders,
    setAvailableArtifactTypes,
  } = useArtifactFilters();

  const parent = isRootInvocation(invocation)
    ? `${invocation.name}/workUnits/root`
    : invocation.name;

  const {
    data: artifactsData,
    isPending: isLoadingArtifacts,
    hasNextPage: artifactsHasNextPage,
    fetchNextPage: loadMoreArtifacts,
  } = useInfiniteQuery({
    ...resultDbClient.ListArtifacts.queryPaged(
      ListArtifactsRequest.fromPartial({
        parent,
        pageSize: 1000,
      }),
    ),
    enabled: !!invocation.name,
    staleTime: Infinity,
    refetchInterval: 10 * 60 * 1000,
    select: selectArtifacts,
  });

  useEffect(() => {
    if (!isLoadingArtifacts && artifactsHasNextPage) {
      loadMoreArtifacts();
    }
  }, [isLoadingArtifacts, loadMoreArtifacts, artifactsHasNextPage]);

  const artifacts = artifactsData || EMPTY_ARRAY;

  // Sync available artifact types to context
  useEffect(() => {
    const types = new Set<string>();
    for (const artifact of artifacts) {
      if (artifact.artifactType) {
        types.add(artifact.artifactType);
      }
    }
    const sortedTypes = Array.from(types).sort();

    setAvailableArtifactTypes(sortedTypes);

    return () => {
      setAvailableArtifactTypes([]);
    };
  }, [artifacts, setAvailableArtifactTypes]);

  const filteredArtifacts = useMemo(
    () =>
      filterArtifacts(artifacts, {
        searchTerm: debouncedSearchTerm,
        artifactTypes,
      }),
    [artifacts, debouncedSearchTerm, artifactTypes],
  );

  const initialArtifactsTree = useMemo(() => {
    // We pass empty array for resultArtifacts as we only have invocation artifacts.
    return buildArtifactsTree(EMPTY_ARRAY, filteredArtifacts, {
      includeSummary:
        isRootInvocation(invocation) && !!invocation.summaryMarkdown,
    });
  }, [filteredArtifacts, invocation]);

  const finalArtifactsTree = useMemo(() => {
    if (!hideEmptyFolders) {
      return initialArtifactsTree;
    }
    return pruneEmptyFolders(initialArtifactsTree);
  }, [initialArtifactsTree, hideEmptyFolders]);

  if (isLoadingArtifacts) {
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
        <Typography sx={{ ml: 1 }}>Loading artifacts...</Typography>
      </Box>
    );
  }

  return (
    <ArtifactsProviderStateManager
      nodes={finalArtifactsTree}
      invocation={invocation}
      debouncedSearchTerm={debouncedSearchTerm}
    >
      {children}
    </ArtifactsProviderStateManager>
  );
}

function ArtifactsProviderStateManager({
  nodes,
  invocation,
  children,
  debouncedSearchTerm,
}: {
  nodes: ReturnType<typeof buildArtifactsTree>;
  invocation: AnyInvocation;
  children: ReactNode;
  debouncedSearchTerm: string;
}) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedNodeId = searchParams.get('artifact') || undefined;

  const setSelectedNode = useCallback(
    (node: ArtifactTreeNodeData | null) => {
      setSearchParams(
        (params) => {
          if (node) {
            params.set('artifact', node.id);
          } else {
            params.delete('artifact');
          }
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  // Auto-selection logic
  useEffect(() => {
    if (debouncedSearchTerm) return;
    if (selectedNodeId) return;

    const summaryNode = nodes.find((node) => node.isSummary);
    if (summaryNode) {
      setSelectedNode(summaryNode);
      return;
    }
    const firstLeaf = findFirstLeafRecursive(nodes);
    if (firstLeaf) {
      setSelectedNode(firstLeaf);
    }
  }, [nodes, selectedNodeId, debouncedSearchTerm, setSelectedNode]);

  return (
    <ArtifactsProvider
      nodes={nodes}
      selectedNodeId={selectedNodeId}
      onSelect={setSelectedNode}
      invocation={invocation}
    >
      {children}
    </ArtifactsProvider>
  );
}
