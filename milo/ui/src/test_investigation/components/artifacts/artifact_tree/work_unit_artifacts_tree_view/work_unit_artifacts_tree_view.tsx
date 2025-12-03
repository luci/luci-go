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
  GetWorkUnitRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';

import { useArtifactsContext } from '../../context';
import { ArtifactTreeNodeData } from '../../types';
import { ArtifactTreeNode } from '../artifact_tree_node';
import { useArtifactFilters } from '../context/context';
import { filterArtifacts } from '../util/tree_util';

function buildWorkUnitTree(
  workUnits: readonly WorkUnit[],
  artifactsByWorkUnit: Record<string, readonly Artifact[]>,
  testResultArtifacts: readonly Artifact[],
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

  // Always add Test Result node
  const filteredTestResultArtifacts = filterArtifactList(testResultArtifacts);
  const testResultNode: ArtifactTreeNodeData = {
    id: 'test_result_root',
    name: 'Test Result',
    children: [],
    isSummary: false,
  };
  for (const art of filteredTestResultArtifacts) {
    // Extract filename from artifact ID (which is a path)
    const parts = art.artifactId.split('/');
    const fileName = parts[parts.length - 1];

    testResultNode.children.push({
      id: art.name,
      name: fileName,
      artifact: art,
      isSummary: false,
      children: [],
      viewingSupported: art.hasLines,
      size: Number(art.sizeBytes),
      url: getRawArtifactURLPath(art.name),
    });
  }
  nodes.push(testResultNode);

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

  // Second pass: Build hierarchy (link work units to parents)
  for (const wu of workUnits) {
    const node = workUnitMap.get(wu.name)!;
    if (wu.parent && workUnitMap.has(wu.parent)) {
      workUnitMap.get(wu.parent)!.children!.push(node);
    } else {
      // If parent not found (or is root invocation), add to top level
      nodes.push(node);
    }
  }

  // Third pass: Add artifacts (so they appear after work units)
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
        viewingSupported: art.hasLines,
        size: Number(art.sizeBytes),
        url: getRawArtifactURLPath(art.name),
      });
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
  testResultArtifacts: readonly Artifact[];
}

function WorkUnitArtifactsTreeViewInternal({
  workUnits,
  artifactsByWorkUnit,
  testResultArtifacts,
}: WorkUnitArtifactsTreeViewInternalProps) {
  const { debouncedSearchTerm, artifactTypes, setAvailableArtifactTypes } =
    useArtifactFilters();
  const { selectedArtifact, setSelectedArtifact: updateSelectedArtifact } =
    useArtifactsContext();

  useEffect(() => {
    const allArtifacts = [
      ...Object.values(artifactsByWorkUnit).flat(),
      ...testResultArtifacts,
    ];
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
  }, [artifactsByWorkUnit, testResultArtifacts, setAvailableArtifactTypes]);

  const treeData = useMemo(() => {
    return buildWorkUnitTree(
      workUnits,
      artifactsByWorkUnit,
      testResultArtifacts,
      (artifacts) =>
        filterArtifacts(artifacts, { debouncedSearchTerm, artifactTypes }),
      debouncedSearchTerm,
    );
  }, [
    workUnits,
    artifactsByWorkUnit,
    testResultArtifacts,
    debouncedSearchTerm,
    artifactTypes,
  ]);

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
  const { currentResult } = useArtifactsContext();

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

  const targetWorkUnitName = `rootInvocations/${rootInvocationId}/workUnits/${workUnitId}`;

  const { data: targetWorkUnitData, isLoading: isLoadingTargetWorkUnit } =
    useQuery(
      resultDbClient.GetWorkUnit.query(
        GetWorkUnitRequest.fromPartial({
          name: targetWorkUnitName,
        }),
      ),
    );

  const {
    data: testResultArtifactsData,
    isLoading: isLoadingTestResultArtifacts,
  } = useQuery(
    resultDbClient.ListArtifacts.query(
      ListArtifactsRequest.fromPartial({
        parent: currentResult?.name,
        pageSize: 1000,
      }),
    ),
  );

  const workUnitArtifactQueries = useQueries({
    queries: [
      ...(workUnitsData?.workUnits || []).map((wu) =>
        resultDbClient.ListArtifacts.query(
          ListArtifactsRequest.fromPartial({
            parent: wu.name,
            pageSize: 1000,
          }),
        ),
      ),
      resultDbClient.ListArtifacts.query(
        ListArtifactsRequest.fromPartial({
          parent: targetWorkUnitName,
          pageSize: 1000,
        }),
      ),
    ],
  });

  const artifactsByWorkUnit = useMemo(() => {
    const map: Record<string, readonly Artifact[]> = {};
    const workUnits = workUnitsData?.workUnits || [];

    // Process ancestors
    workUnits.forEach((wu, index) => {
      const query = workUnitArtifactQueries[index];
      if (wu && query.data) {
        map[wu.name] = query.data.artifacts || [];
      }
    });

    // Process target work unit (last query)
    const targetQuery = workUnitArtifactQueries[workUnits.length];
    if (targetQuery && targetQuery.data) {
      map[targetWorkUnitName] = targetQuery.data.artifacts || [];
    }

    return map;
  }, [workUnitArtifactQueries, workUnitsData, targetWorkUnitName]);

  const isLoadingWorkUnitArtifacts = workUnitArtifactQueries.some(
    (q) => q.isLoading,
  );
  const isLoading =
    isLoadingWorkUnits ||
    isLoadingWorkUnitArtifacts ||
    isLoadingTargetWorkUnit ||
    isLoadingTestResultArtifacts;
  const workUnits = useMemo(() => {
    const units = [...(workUnitsData?.workUnits || [])];
    if (units.length > 0 || workUnitsData) {
      const parentName =
        targetWorkUnitData?.parent ||
        (units.length > 0 ? units[units.length - 1].name : undefined);

      units.push(
        WorkUnit.fromPartial({
          name: targetWorkUnitName,
          workUnitId: workUnitId,
          parent: parentName,
          kind: targetWorkUnitData?.kind,
        }),
      );
    }
    return units;
  }, [workUnitsData, targetWorkUnitName, workUnitId, targetWorkUnitData]);

  if (isLoading) {
    return <CircularProgress />;
  }

  return (
    <WorkUnitArtifactsTreeViewInternal
      workUnits={workUnits}
      artifactsByWorkUnit={artifactsByWorkUnit}
      testResultArtifacts={testResultArtifactsData?.artifacts || []}
    />
  );
}
