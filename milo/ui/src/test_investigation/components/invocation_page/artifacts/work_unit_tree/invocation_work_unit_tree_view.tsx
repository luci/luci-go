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

import { useMemo, useEffect } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { useArtifacts } from '@/test_investigation/components/common/artifacts/context';
import { WorkUnitArtifactTree } from '@/test_investigation/components/common/artifacts/tree/work_unit_artifact_tree';
import { ArtifactTreeNodeData } from '@/test_investigation/components/common/artifacts/types';

import { useAllInvocationArtifacts, useInvocationWorkUnitTree } from './hooks';

interface InvocationWorkUnitTreeViewProps {
  rootInvocationId: string;
  onNodeSelect?: (node: ArtifactTreeNodeData | null) => void;
}

export function InvocationWorkUnitTreeView({
  rootInvocationId,
  onNodeSelect,
}: InvocationWorkUnitTreeViewProps) {
  const { onSelect: updateSelectedArtifact } = useArtifacts();
  const [searchParams] = useSyncedSearchParams();
  const selectedArtifactId = searchParams.get('artifact');

  const { workUnits, isLoading: isLoadingWorkUnits } =
    useInvocationWorkUnitTree(rootInvocationId);

  const {
    data: artifactsData,
    isLoading: isLoadingArtifacts,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useAllInvocationArtifacts(rootInvocationId);

  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const artifactsByWorkUnit = useMemo(() => {
    const map: Record<string, Artifact[]> = {};
    if (!artifactsData) return map;

    for (const page of artifactsData.pages) {
      if (!page) continue;
      for (const artifact of page.artifacts || []) {
        // Parse WorkUnit name from Artifact name
        // Format: rootInvocations/{RID}/workUnits/{WUID}/artifacts/{AID}
        const parts = artifact.name.split('/artifacts/');
        if (parts.length < 2) continue;
        const workUnitName = parts[0];

        if (!map[workUnitName]) {
          map[workUnitName] = [];
        }
        map[workUnitName].push(artifact);
      }
    }
    return map;
  }, [artifactsData]);

  const isLoading =
    isLoadingWorkUnits || isLoadingArtifacts || isFetchingNextPage;

  return (
    <WorkUnitArtifactTree
      workUnits={workUnits}
      artifactsByWorkUnit={artifactsByWorkUnit}
      selectedNodeId={selectedArtifactId}
      isLoading={isLoading}
      onNodeSelect={(node) => {
        onNodeSelect?.(node);
        if (node) {
          updateSelectedArtifact(node);
        }
      }}
    />
  );
}
