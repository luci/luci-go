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

import { CircularProgress, Typography } from '@mui/material';
import { useQueries, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { WorkUnitPredicate } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { ListArtifactsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  QueryWorkUnitsRequest,
  WorkUnit,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';

import { useArtifactsContext } from '../../context';
import { ArtifactTreeNodeData } from '../../types';
import { ArtifactTreeNode } from '../artifact_tree_node';
import { useArtifactFilters } from '../context/context';
import { filterArtifacts } from '../util/tree_util';

function buildWorkUnitTree(
  workUnits: readonly WorkUnit[],
  artifactsByWorkUnit: Record<string, readonly Artifact[]>,
  filterArtifactList: (artifacts: readonly Artifact[]) => readonly Artifact[],
  debouncedSearchTerm: string,
): ArtifactTreeNodeData[] {
  const nodes: ArtifactTreeNodeData[] = [];
  const workUnitMap = new Map<string, ArtifactTreeNodeData>();

  nodes.push({
    id: 'summary_node',
    name: 'Summary',
    isSummary: true,
    children: [],
  });

  // First pass: Create nodes for all work units
  for (const wu of workUnits) {
    const node: ArtifactTreeNodeData = {
      id: wu.name,
      name: wu.kind || wu.workUnitId,
      children: [],
      isSummary: false,
    };
    workUnitMap.set(wu.name, node);
  }

  // Second pass: Build hierarchy and add artifacts
  for (const wu of workUnits) {
    const node = workUnitMap.get(wu.name)!;

    // Add artifacts
    const artifacts = artifactsByWorkUnit[wu.name] || [];
    // Filter artifacts using the context helper
    const filteredArtifacts = filterArtifactList(artifacts);

    for (const art of filteredArtifacts) {
      // Extract filename from artifact ID (which is a path)
      const parts = art.artifactId.split('/');
      const fileName = parts[parts.length - 1];

      node.children!.push({
        id: art.name,
        name: fileName,
        artifact: art,
        isSummary: false,
        children: [],
      });
    }

    // Attach to parent if exists in map
    if (wu.parent && workUnitMap.has(wu.parent)) {
      workUnitMap.get(wu.parent)!.children!.push(node);
    } else {
      // If parent not found (or is root invocation), add to top level
      nodes.push(node);
    }
  }

  if (!debouncedSearchTerm) {
    return nodes;
  }

  const prune = (nodes: ArtifactTreeNodeData[]): ArtifactTreeNodeData[] => {
    const result: ArtifactTreeNodeData[] = [];
    for (const node of nodes) {
      if (node.isSummary) {
        if (
          node.name.toLowerCase().includes(debouncedSearchTerm.toLowerCase())
        ) {
          result.push(node);
        }
        continue;
      }

      const matchesSelf = node.name
        .toLowerCase()
        .includes(debouncedSearchTerm.toLowerCase());
      const prunedChildren = prune(node.children || []);

      if (matchesSelf || prunedChildren.length > 0) {
        result.push({ ...node, children: prunedChildren });
      }
    }
    return result;
  };

  return prune(nodes);
}

interface WorkUnitArtifactsTreeViewProps {
  rootInvocationId: string;
  workUnitId: string;
}

interface WorkUnitArtifactsTreeViewInternalProps {
  workUnits: readonly WorkUnit[];
  artifactsByWorkUnit: Record<string, readonly Artifact[]>;
}

function WorkUnitArtifactsTreeViewInternal({
  workUnits,
  artifactsByWorkUnit,
}: WorkUnitArtifactsTreeViewInternalProps) {
  const { debouncedSearchTerm, artifactTypes, setAvailableArtifactTypes } =
    useArtifactFilters();
  const { selectedArtifact, setSelectedArtifact: updateSelectedArtifact } =
    useArtifactsContext();

  // Sync available artifact types to context
  useEffect(() => {
    const allArtifacts = Object.values(artifactsByWorkUnit).flat();
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
  }, [artifactsByWorkUnit, setAvailableArtifactTypes]);

  const treeData = useMemo(() => {
    return buildWorkUnitTree(
      workUnits,
      artifactsByWorkUnit,
      (artifacts) =>
        filterArtifacts(artifacts, { debouncedSearchTerm, artifactTypes }),
      debouncedSearchTerm,
    );
  }, [workUnits, artifactsByWorkUnit, debouncedSearchTerm, artifactTypes]);

  const selectedNodes = useMemo(() => {
    return selectedArtifact ? new Set([selectedArtifact.id]) : undefined;
  }, [selectedArtifact]);

  const setActiveSelectionFn = (node: ArtifactTreeNodeData) => {
    return selectedArtifact?.id === node.id;
  };

  if (treeData.length === 0) {
    return <Typography color="text.secondary">No work units found.</Typography>;
  }

  return (
    <VirtualTree<ArtifactTreeNodeData>
      root={treeData}
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
          onSupportedLeafClick={(node) => updateSelectedArtifact(node)}
          onUnsupportedLeafClick={(node) => {
            open(getRawArtifactURLPath(node.artifact?.name || ''), '_blank');
          }}
          highlightText={debouncedSearchTerm}
        />
      )}
      selectedNodes={selectedNodes}
      setActiveSelectionFn={setActiveSelectionFn}
    />
  );
}

export function WorkUnitArtifactsTreeView({
  rootInvocationId,
  workUnitId,
}: WorkUnitArtifactsTreeViewProps) {
  const resultDbClient = useResultDbClient();

  const { data: workUnitsData, isLoading: isLoadingWorkUnits } = useQuery(
    resultDbClient.QueryWorkUnits.query(
      QueryWorkUnitsRequest.fromPartial({
        parent: `rootInvocations/${rootInvocationId}`,
        predicate: WorkUnitPredicate.fromPartial({
          ancestorsOf: `rootInvocations/${rootInvocationId}/workUnits/${workUnitId}`,
        }),
      }),
    ),
  );

  const workUnitArtifactQueries = useQueries({
    queries: (workUnitsData?.workUnits || []).map((wu) =>
      resultDbClient.ListArtifacts.query(
        ListArtifactsRequest.fromPartial({
          parent: wu.name,
          pageSize: 1000,
        }),
      ),
    ),
  });

  const artifactsByWorkUnit = useMemo(() => {
    const map: Record<string, readonly Artifact[]> = {};
    workUnitArtifactQueries.forEach((query, index) => {
      const wu = workUnitsData?.workUnits[index];
      if (wu && query.data) {
        map[wu.name] = query.data.artifacts || [];
      }
    });
    return map;
  }, [workUnitArtifactQueries, workUnitsData]);

  const isLoadingWorkUnitArtifacts = workUnitArtifactQueries.some(
    (q) => q.isLoading,
  );
  const isLoading = isLoadingWorkUnits || isLoadingWorkUnitArtifacts;
  const workUnits = useMemo(
    () => workUnitsData?.workUnits || [],
    [workUnitsData],
  );

  if (isLoading) {
    return <CircularProgress />;
  }

  return (
    <WorkUnitArtifactsTreeViewInternal
      workUnits={workUnits}
      artifactsByWorkUnit={artifactsByWorkUnit}
    />
  );
}
