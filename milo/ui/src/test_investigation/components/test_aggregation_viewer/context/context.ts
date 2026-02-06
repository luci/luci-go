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

import { TestAggregation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { TestVerdict } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

export interface BaseAggregationNode {
  id: string; // Unique ID for the node (e.g. prefix)
  label: string;
  depth: number;
}

export interface IntermediateAggregationNode extends BaseAggregationNode {
  isLeaf: false;
  aggregationData?: TestAggregation;
  children?: AggregationNode[];
  isLoading?: boolean;
  hasLoadError?: boolean;
}

export interface LeafAggregationNode extends BaseAggregationNode {
  isLeaf: true;
  verdict?: TestVerdict;
}

export type AggregationNode = IntermediateAggregationNode | LeafAggregationNode;

export interface TestAggregationContextValue {
  expandedIds: Set<string>;
  toggleExpansion: (id: string) => void;
  flattenedItems: AggregationNode[];
  isLoading: boolean;
  highlightedNodeId?: string;
  selectedStatuses: Set<string>;
  setSelectedStatuses: (statuses: Set<string>) => void;
  /**
   * Signal to scroll to a specific node.
   * The timestamp ensures that even if the ID is the same, we can trigger a re-scroll.
   */
  scrollRequest?: { id: string; ts: number };
  locateCurrentTest: () => void;
}

export const TestAggregationContext =
  createContext<TestAggregationContextValue | null>(null);

export function useTestAggregationContext() {
  const context = useContext(TestAggregationContext);
  if (!context) {
    throw new Error(
      'useTestAggregation must be used within a TestAggregationProvider',
    );
  }
  return context;
}
