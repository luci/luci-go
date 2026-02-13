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
import { useMemo, useEffect, useRef } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { WorkUnit } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';
import { ArtifactTreeNode } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_node/artifact_tree_node';
import { useArtifactFilters } from '@/test_investigation/components/common/artifacts/tree/context/context';
import {
  filterArtifacts,
  findNode,
  getWorkUnitLabel,
} from '@/test_investigation/components/common/artifacts/tree/util/tree_util';
import { ArtifactTreeNodeData } from '@/test_investigation/components/common/artifacts/types';
import { useInvocation } from '@/test_investigation/context/context';
import {
  isRootInvocation,
  AnyInvocation,
} from '@/test_investigation/utils/invocation_utils';

interface WorkUnitArtifactTreeProps {
  workUnits: readonly WorkUnit[];
  artifactsByWorkUnit: Record<string, readonly Artifact[]>;
  testResultArtifacts?: readonly Artifact[];
  selectedNodeId?: string | null;
  onNodeSelect?: (node: ArtifactTreeNodeData | null) => void;
  isLoading?: boolean;
}

function buildWorkUnitTree(
  workUnits: readonly WorkUnit[],
  artifactsByWorkUnit: Record<string, readonly Artifact[]>,
  testResultArtifacts: readonly Artifact[],
  filterArtifactList: (artifacts: readonly Artifact[]) => readonly Artifact[],
  debouncedSearchTerm: string,
  invocation: AnyInvocation | null,
): ArtifactTreeNodeData[] {
  const nodes: ArtifactTreeNodeData[] = [];
  const workUnitMap = new Map<string, ArtifactTreeNodeData>();

  nodes.push({
    id: 'summary_node',
    name: 'Summary',
    isSummary: true,
    children: [],
  });

  // Always add Test Result node if artifacts exist
  if (testResultArtifacts.length > 0) {
    const filteredTestResultArtifacts = filterArtifactList(testResultArtifacts);

    const testResultNode: ArtifactTreeNodeData = {
      id: 'test_result_root',
      name: 'Test Result',
      children: [],
      isSummary: false,
    };
    for (const art of filteredTestResultArtifacts) {
      const parts = art.artifactId.split('/');
      const fileName = parts[parts.length - 1];

      testResultNode.children!.push({
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
  }

  // First pass: Create nodes for all work units
  for (const wu of workUnits) {
    const name = getWorkUnitLabel(
      wu,
      invocation && isRootInvocation(invocation)
        ? invocation.definition?.name
        : undefined,
    );

    const node: ArtifactTreeNodeData = {
      id: wu.name,
      name,
      children: [],
      isSummary: false,
    };
    workUnitMap.set(wu.name, node);
  }

  // Second pass: Add artifacts (so they appear before child work units)
  for (const wu of workUnits) {
    const node = workUnitMap.get(wu.name)!;

    const artifacts = artifactsByWorkUnit[wu.name] || [];
    const filteredArtifacts = filterArtifactList(artifacts);

    for (const art of filteredArtifacts) {
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

  // Third pass: Build hierarchy (link work units to parents)
  for (const wu of workUnits) {
    const node = workUnitMap.get(wu.name)!;
    if (wu.parent && workUnitMap.has(wu.parent)) {
      workUnitMap.get(wu.parent)!.children!.push(node);
    } else {
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

function pruneEmptyFolders(
  nodes: ArtifactTreeNodeData[],
): ArtifactTreeNodeData[] {
  const result: ArtifactTreeNodeData[] = [];
  for (const node of nodes) {
    if (node.isSummary) {
      result.push(node);
      continue;
    }
    if (node.artifact) {
      result.push(node);
      continue;
    }

    const prunedChildren = pruneEmptyFolders(node.children || []);

    if (prunedChildren.length > 0) {
      result.push({ ...node, children: prunedChildren });
    }
  }
  return result;
}

export function WorkUnitArtifactTree({
  workUnits,
  artifactsByWorkUnit,
  testResultArtifacts = [],
  selectedNodeId,
  onNodeSelect,
  isLoading,
}: WorkUnitArtifactTreeProps) {
  const invocation = useInvocation();
  const {
    debouncedSearchTerm,
    artifactTypes,
    hideEmptyFolders,
    setAvailableArtifactTypes,
  } = useArtifactFilters();

  // Consolidate artifact types logic for filtering
  const lastAvailableArtifactTypes = useRef<string[]>([]);
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
    const newTypes = Array.from(types).sort();

    // Deep compare with last set types
    const prev = lastAvailableArtifactTypes.current;
    if (
      prev.length === newTypes.length &&
      prev.every((t, i) => t === newTypes[i])
    ) {
      return;
    }

    lastAvailableArtifactTypes.current = newTypes;
    setAvailableArtifactTypes(newTypes);
  }, [artifactsByWorkUnit, testResultArtifacts, setAvailableArtifactTypes]);

  const treeData = useMemo(() => {
    const newTree = buildWorkUnitTree(
      workUnits,
      artifactsByWorkUnit,
      testResultArtifacts,
      (artifacts) =>
        filterArtifacts(artifacts, {
          searchTerm: debouncedSearchTerm,
          artifactTypes,
        }),
      debouncedSearchTerm,
      invocation,
    );

    if (hideEmptyFolders) {
      return pruneEmptyFolders(newTree);
    }
    return newTree;
  }, [
    workUnits,
    artifactsByWorkUnit,
    testResultArtifacts,
    debouncedSearchTerm,
    artifactTypes,
    invocation,
    hideEmptyFolders,
  ]);

  const selectedNode = useMemo(() => {
    if (!selectedNodeId) return null;
    return findNode(treeData, selectedNodeId);
  }, [treeData, selectedNodeId]);

  const selectedNodes = useMemo(() => {
    return selectedNode ? new Set([selectedNode.id]) : undefined;
  }, [selectedNode]);

  // Sync parent state when a valid node is selected via ID (deep linking)
  const lastNotifiedNodeId = useRef<string | null>(null);
  useEffect(() => {
    if (selectedNode && selectedNode.id !== lastNotifiedNodeId.current) {
      onNodeSelect?.(selectedNode);
      lastNotifiedNodeId.current = selectedNode.id;
    }
  }, [selectedNode, onNodeSelect]);

  const setActiveSelectionFn = (node: ArtifactTreeNodeData) => {
    return selectedNode?.id === node.id;
  };

  // We should probably rely on parent to handle major loading.
  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
        <CircularProgress size={24} />
      </Box>
    );
  }

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
          onSupportedLeafClick={(node) => onNodeSelect?.(node)}
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
