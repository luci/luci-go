// Copyright 2026 The LUCI Authors.
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

import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestAggregation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { TestVerdict } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

export type VerdictCounts = TestAggregation['verdictCounts'];

export interface BaseAggregationNode {
  id: string; // Unique ID for the node (e.g. prefix)
  label: string;
  depth: number;
  labelParts?: {
    key: string;
    value: string;
  };
}

export interface IntermediateAggregationNode extends BaseAggregationNode {
  isLeaf: false;
  isLoadMore: false;
  aggregationData?: TestAggregation;
  children?: AggregationNode[];
  isLoading?: boolean;
  hasLoadError?: boolean;
  nextFinerLevel?: AggregationLevel;
}

export interface LeafAggregationNode extends BaseAggregationNode {
  isLeaf: true;
  isLoadMore: false;
  verdict?: TestVerdict;
}

export interface LoadMoreAggregationNode extends BaseAggregationNode {
  isLeaf: true;
  isLoadMore: true;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
}

export type AggregationNode =
  | IntermediateAggregationNode
  | LeafAggregationNode
  | LoadMoreAggregationNode;

export interface AggregationViewContextValue {
  // Tree State
  expandedIds: Set<string>;
  toggleExpansion: (id: string) => void;
  flattenedItems: AggregationNode[];
  invocation: AnyInvocation;
  setNodeChildren: (
    nodeId: string,
    items: AggregationNode[],
    hasNextPage: boolean,
    isFetchingNextPage: boolean,
    fetchNextPage: () => void,
  ) => void;

  // Loading State
  isLoading: boolean;
  isError: boolean;
  error: unknown;

  // Interaction
  scrollRequest?: { id: string; ts: number };
  locateCurrentTest: () => void;
  highlightedNodeId?: string;
}

export const AggregationViewContext =
  createContext<AggregationViewContextValue | null>(null);

export function useAggregationViewContext() {
  const ctx = useContext(AggregationViewContext);
  if (!ctx) {
    throw new Error(
      'useAggregationViewContext must be used within an AggregationViewProvider',
    );
  }
  return ctx;
}
