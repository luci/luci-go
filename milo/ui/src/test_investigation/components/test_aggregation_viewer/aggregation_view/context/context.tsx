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

export interface AggregationViewContextValue {
  // Tree State
  expandedIds: Set<string>;
  toggleExpansion: (id: string) => void;
  flattenedItems: AggregationNode[];

  // Loading State
  isLoading: boolean;

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
