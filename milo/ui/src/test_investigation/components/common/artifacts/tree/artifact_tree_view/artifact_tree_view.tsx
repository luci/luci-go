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

import { useCallback, useMemo } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';

import { useArtifacts } from '../../context/context';
import { ArtifactTreeNodeData } from '../../types';
import { ArtifactTreeNode } from '../artifact_tree_node';
import { useArtifactFilters } from '../context/context';

export function ArtifactTreeView() {
  const { debouncedSearchTerm } = useArtifactFilters();
  const { nodes, selectedNode, onSelect } = useArtifacts();

  const setActiveSelectionFnForSelectedNode = useCallback(
    (nodeData: ArtifactTreeNodeData): boolean => {
      if (!selectedNode) return false;
      if (selectedNode.isSummary) return !!nodeData.isSummary;
      return (
        nodeData.artifact?.artifactId === selectedNode.artifact?.artifactId
      );
    },
    [selectedNode],
  );

  const selectedNodes: Set<string> | undefined = useMemo(() => {
    if (selectedNode) {
      return new Set([selectedNode.id]);
    }
    return undefined;
  }, [selectedNode]);

  function handleLeafNodeClicked(node: ArtifactTreeNodeData) {
    if (node.artifact || node.isSummary) {
      onSelect(node);
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
      root={nodes}
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
