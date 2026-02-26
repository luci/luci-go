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

import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { Scheme } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/schema.pb';
import { TestAggregation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import {
  TestVerdict,
  TestVerdict_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { getTestIdentifierPrefixId } from '../../utils';

import { AggregationNode } from './context';

export function buildAggregationFilterString(
  selectedStatuses: Set<string>,
): string {
  if (selectedStatuses.size === 0) {
    // Default fallback
    return '';
  }
  const parts: string[] = [];
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.FAILED])) {
    parts.push('matched_verdict_counts.failed > 0');
  }
  if (
    selectedStatuses.has(
      TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
    )
  ) {
    parts.push('matched_verdict_counts.execution_errored > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.FLAKY])) {
    parts.push('matched_verdict_counts.flaky > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.PASSED])) {
    parts.push('matched_verdict_counts.passed > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.SKIPPED])) {
    parts.push('matched_verdict_counts.skipped > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.PRECLUDED])) {
    parts.push('matched_verdict_counts.precluded > 0');
  }
  if (selectedStatuses.has('EXONERATED')) {
    parts.push('matched_verdict_counts.exonerated > 0');
  }
  return parts.length > 0 ? parts.join(' OR ') : 'false';
}

// Helper to resolve Label and Parts
export function resolveLabel(
  node: AggregationNode,
  schemes: { [key: string]: Scheme },
): { label: string; labelParts?: { key: string; value: string } } {
  // If verdict, use testId
  if (node.isLeaf && !node.isLoadMore && node.verdict) {
    return {
      label: node.verdict.testIdStructured?.caseName || node.verdict.testId,
    };
  }

  // If aggregation
  if (!node.isLeaf && !node.isLoadMore && node.aggregationData) {
    const prefix = node.aggregationData.id!;
    const ident = prefix.id;
    const schemeId = ident?.moduleScheme || '';
    const scheme = schemes[schemeId];

    let prefixLabel = '';
    let name = '';

    if (prefix.level === AggregationLevel.MODULE) {
      prefixLabel = 'Module';
      name = ident?.moduleName || '';
    } else if (prefix.level === AggregationLevel.COARSE) {
      prefixLabel = scheme?.coarse?.humanReadableName || 'Coarse';
      name = ident?.coarseName || '';
    } else if (prefix.level === AggregationLevel.FINE) {
      prefixLabel = scheme?.fine?.humanReadableName || 'Fine';
      name = ident?.fineName || '';
    }

    if (prefixLabel && name) {
      return {
        label: `${prefixLabel}: ${name}`,
        labelParts: { key: prefixLabel, value: name },
      };
    }
    return { label: name || prefixLabel || node.id };
  }

  return { label: node.label || node.id };
}

export function mapAggregationToNode(
  agg: TestAggregation,
  schemes: { [key: string]: Scheme },
): AggregationNode {
  const node: AggregationNode = {
    id: getTestIdentifierPrefixId(agg.id!),
    label: '',
    depth: 0,
    isLeaf: false,
    isLoadMore: false,
    aggregationData: agg,
    nextFinerLevel: agg.nextFinerLevel,
  };
  const { label, labelParts } = resolveLabel(node, schemes);
  node.label = label;
  if (labelParts) {
    node.labelParts = labelParts;
  }
  return node;
}

export function mapVerdictToNode(
  v: TestVerdict,
  schemes: { [key: string]: Scheme },
): AggregationNode {
  const node: AggregationNode = {
    id: v.testId,
    label: '',
    depth: 0,
    isLeaf: true,
    isLoadMore: false,
    verdict: v,
  };
  const { label, labelParts } = resolveLabel(node, schemes);
  node.label = label;
  if (labelParts) {
    node.labelParts = labelParts;
  }
  return node;
}

export interface NodeChildrenState {
  items: AggregationNode[];
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
}

// Merge and Flatten dynamic tree
export function flattenDynamicTree(
  rootNodes: AggregationNode[],
  childrenMap: Map<string, NodeChildrenState>,
  expandedIds: Set<string>,
): AggregationNode[] {
  const flatList: AggregationNode[] = [];

  const flatten = (node: AggregationNode, depth: number) => {
    const cloned = { ...node, depth };
    flatList.push(cloned);

    if (expandedIds.has(node.id)) {
      const childrenState = childrenMap.get(node.id);
      if (childrenState) {
        childrenState.items.forEach((child) => flatten(child, depth + 1));

        if (childrenState.hasNextPage) {
          flatList.push({
            id: `${node.id}-load-more`,
            label: 'Load more',
            depth: depth + 1,
            isLeaf: true,
            isLoadMore: true,
            hasNextPage: childrenState.hasNextPage,
            isFetchingNextPage: childrenState.isFetchingNextPage,
            fetchNextPage: childrenState.fetchNextPage,
          });
        }
      }
    }
  };

  rootNodes.forEach((r) => flatten(r, 0));
  return flatList;
}
