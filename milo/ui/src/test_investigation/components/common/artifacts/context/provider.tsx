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

import { ReactNode, useMemo, useState } from 'react';

import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { ArtifactTreeNodeData } from '../types';

import { ArtifactsContext } from './context';

export interface ArtifactsProviderProps {
  readonly children: ReactNode;
  readonly nodes: ArtifactTreeNodeData[];
  readonly selectedNodeId?: string;
  readonly onSelect?: (node: ArtifactTreeNodeData) => void;
  readonly invocation?: AnyInvocation;
}

export function ArtifactsProvider({
  nodes,
  children,
  selectedNodeId,
  onSelect,
  invocation,
}: ArtifactsProviderProps) {
  const [filterQuery, setFilterQuery] = useState('');

  const contextValue = useMemo(() => {
    // We need to find the selected node object from ID if only ID is passed?
    // Or we assume `selectedNode` prop is passed?
    // The requirement says "The component needs to be generic".
    // I made `selectedNodeId` prop, but context returns `selectedNode`.

    const findNode = (
      nodes: ArtifactTreeNodeData[],
      id: string,
    ): ArtifactTreeNodeData | null => {
      for (const node of nodes) {
        if (node.id === id) return node;
        if (node.children) {
          const found = findNode(node.children, id);
          if (found) return found;
        }
      }
      return null;
    };

    const selectedNode = selectedNodeId
      ? findNode(nodes, selectedNodeId)
      : null;

    return {
      nodes,
      selectedNode,
      onSelect: (node: ArtifactTreeNodeData) => onSelect?.(node),
      filterQuery,
      matchedNodeId: '',
      onFilterQueryChange: setFilterQuery,
      onNextMatch: () => {},
      onPrevMatch: () => {},
      matchStatus: '',
      invocation,
    };
  }, [nodes, selectedNodeId, onSelect, filterQuery, invocation]);

  return (
    <ArtifactsContext.Provider value={contextValue}>
      {children}
    </ArtifactsContext.Provider>
  );
}
