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

import { createContext, useContext } from 'react';

import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { ArtifactTreeNodeData } from '../types';

export interface ArtifactsContextV2 {
  /**
   * The tree nodes to display.
   */
  readonly nodes: ArtifactTreeNodeData[];

  /**
   * The currently selected artifact node.
   */
  readonly selectedNode: ArtifactTreeNodeData | null;

  /**
   * Callback when a node is selected.
   */
  readonly onSelect: (node: ArtifactTreeNodeData) => void;

  /**
   * Current filter query.
   */
  readonly filterQuery: string;

  /**
   * ID of the tree node matched by the filter.
   */
  readonly matchedNodeId: string;

  /**
   * Callback when filter query changes.
   */
  readonly onFilterQueryChange: (query: string) => void;

  /**
   * Callback when next match is requested.
   */
  readonly onNextMatch: () => void;

  /**
   * Callback when previous match is requested.
   */
  readonly onPrevMatch: () => void;

  /**
   * Current match index (1-based) / Total matches.
   */
  readonly matchStatus: string;

  /**
   * The invocation associated with the artifacts (optional).
   * Needed for things like bug links.
   */
  readonly invocation?: AnyInvocation;
}

export const ArtifactsContext = createContext<ArtifactsContextV2 | null>(null);

export function useArtifacts() {
  const context = useContext(ArtifactsContext);
  if (!context) {
    throw new Error('useArtifacts must be used within an ArtifactsProvider');
  }
  return context;
}
