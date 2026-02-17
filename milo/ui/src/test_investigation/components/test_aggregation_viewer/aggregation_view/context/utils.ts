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

import { createPrefixForTest, getTestIdentifierPrefixId } from '../../utils';

import { AggregationNode } from './context';

export function buildAggregationFilterString(
  selectedStatuses: Set<string>,
): string {
  if (selectedStatuses.size === 0) {
    // Default fallback
    return 'verdict_counts.failed > 0 OR verdict_counts.execution_errored > 0 OR verdict_counts.flaky > 0';
  }
  const parts: string[] = [];
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.FAILED])) {
    parts.push('verdict_counts.failed > 0');
  }
  if (
    selectedStatuses.has(
      TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
    )
  ) {
    parts.push('verdict_counts.execution_errored > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.FLAKY])) {
    parts.push('verdict_counts.flaky > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.PASSED])) {
    parts.push('verdict_counts.passed > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.SKIPPED])) {
    parts.push('verdict_counts.skipped > 0');
  }
  if (selectedStatuses.has(TestVerdict_Status[TestVerdict_Status.PRECLUDED])) {
    parts.push('verdict_counts.precluded > 0');
  }
  if (selectedStatuses.has('EXONERATED')) {
    parts.push('verdict_counts.exonerated > 0');
  }
  return parts.length > 0 ? parts.join(' OR ') : 'false';
}

interface EnrichmentQuery {
  data?: {
    readonly aggregations?: readonly TestAggregation[];
  };
}

// Helper to register node
export function registerNode(
  nodes: Map<string, AggregationNode>,
  childrenMap: Map<string, AggregationNode[]>,
  node: AggregationNode,
  parentId?: string,
) {
  if (!nodes.has(node.id)) {
    nodes.set(node.id, node);
  } else {
    // Merge updates
    const existing = nodes.get(node.id)!;
    if (!node.isLeaf && node.aggregationData && !existing.isLeaf) {
      existing.aggregationData = node.aggregationData;
    }
    if (node.isLeaf && node.verdict && existing.isLeaf) {
      existing.verdict = node.verdict;
    }
  }

  if (parentId) {
    if (!childrenMap.has(parentId)) {
      childrenMap.set(parentId, []);
    }
    const list = childrenMap.get(parentId)!;
    if (!list.find((n) => n.id === node.id)) {
      list.push(nodes.get(node.id)!);
    }
  }
}

// Helper to resolve Label
export function resolveLabel(
  node: AggregationNode,
  schemes: { [key: string]: Scheme },
): string {
  // If verdict, use testId
  if (node.isLeaf && node.verdict) {
    return node.verdict.testIdStructured?.caseName || node.verdict.testId;
  }

  // If aggregation
  if (!node.isLeaf && node.aggregationData) {
    const prefix = node.aggregationData.id!;
    const ident = prefix.id;
    const schemeId = ident?.moduleScheme || '';
    const scheme = schemes[schemeId];

    let prefixLabel = '';
    let name = '';

    if (prefix.level === AggregationLevel.MODULE) {
      name = `Module: ${ident?.moduleName || ''}`;
      return name;
    } else if (prefix.level === AggregationLevel.COARSE) {
      prefixLabel = scheme?.coarse?.humanReadableName || 'Coarse';
      name = ident?.coarseName || '';
    } else if (prefix.level === AggregationLevel.FINE) {
      prefixLabel = scheme?.fine?.humanReadableName || 'Fine';
      name = ident?.fineName || '';
    }

    return `${prefixLabel}: ${name}`;
  }

  return node.label || node.id;
}

// Helper to process a single verdict
// Uses top-down logic to ensure parent nodes are registered in order
function processVerdict(
  v: TestVerdict,
  nodes: Map<string, AggregationNode>,
  childrenMap: Map<string, AggregationNode[]>,
) {
  const testId = v.testIdStructured;
  if (!testId) {
    return;
  }

  // 1. Identify Hierarchy (Top-Down)
  const modulePrefix = createPrefixForTest(
    { testIdStructured: testId },
    AggregationLevel.MODULE,
  );
  const moduleId = getTestIdentifierPrefixId(modulePrefix);

  // Ensure Module exists
  registerNode(nodes, childrenMap, {
    id: moduleId,
    label: testId.moduleName || 'Module',
    isLeaf: false,
    depth: 0,
    isLoading: true,
  });

  let parentId = moduleId;

  // Coarse
  if (testId.coarseName) {
    const coarsePrefix = createPrefixForTest(
      { testIdStructured: testId },
      AggregationLevel.COARSE,
    );
    const coarseId = getTestIdentifierPrefixId(coarsePrefix);
    registerNode(
      nodes,
      childrenMap,
      {
        id: coarseId,
        label: testId.coarseName,
        isLeaf: false,
        depth: 0,
        isLoading: true,
      },
      parentId,
    );
    parentId = coarseId;
  }

  // Fine
  if (testId.fineName) {
    const finePrefix = createPrefixForTest(
      { testIdStructured: testId },
      AggregationLevel.FINE,
    );
    const fineId = getTestIdentifierPrefixId(finePrefix);
    registerNode(
      nodes,
      childrenMap,
      {
        id: fineId,
        label: testId.fineName,
        isLeaf: false,
        depth: 0,
        isLoading: true,
      },
      parentId,
    );
    parentId = fineId;
  }

  // 2. Case Node (Bottom)
  const caseId = v.testId;

  registerNode(
    nodes,
    childrenMap,
    {
      id: caseId,
      verdict: v,
      label: v.testId,
      isLeaf: true,
      depth: 0,
    },
    parentId,
  );
}

// 1. Build Skeleton from Verdicts
export function buildSkeleton(verdicts: readonly TestVerdict[] | undefined): {
  nodes: Map<string, AggregationNode>;
  childrenMap: Map<string, AggregationNode[]>;
} {
  const nodes = new Map<string, AggregationNode>();
  const childrenMap = new Map<string, AggregationNode[]>();

  if (!verdicts) {
    return { nodes, childrenMap };
  }
  verdicts.forEach((v) => {
    processVerdict(v, nodes, childrenMap);
  });
  return { nodes, childrenMap };
}

// 2. Process Aggregations (Enrichment)
export function processAggregations(enrichmentQuery: EnrichmentQuery[]): {
  dataMap: Map<string, TestAggregation>;
} {
  const dataMap = new Map<string, TestAggregation>();

  enrichmentQuery.forEach((q) => {
    q.data?.aggregations?.forEach((agg) => {
      const id = getTestIdentifierPrefixId(agg.id!);
      dataMap.set(id, agg);
    });
  });
  return { dataMap };
}

// 3. Merge and Flatten
export function mergeAndFlatten(
  nodes: Map<string, AggregationNode>,
  childrenMap: Map<string, AggregationNode[]>,
  aggDataMap: Map<string, TestAggregation>,
  schemes: { [key: string]: Scheme },
  expandedIds: Set<string>,
): AggregationNode[] {
  const flatList: AggregationNode[] = [];

  const flatten = (nodeId: string, depth: number) => {
    const original = nodes.get(nodeId);
    if (!original) {
      return;
    }

    // Clone and Merge
    const node = { ...original };
    node.depth = depth;

    if (!node.isLeaf && aggDataMap.has(nodeId)) {
      node.aggregationData = aggDataMap.get(nodeId);
      node.isLoading = false;
    }

    node.label = resolveLabel(node, schemes);
    flatList.push(node);

    if (expandedIds.has(nodeId)) {
      const children = childrenMap.get(nodeId) || [];
      children.sort((a, b) => a.label.localeCompare(b.label));
      children.forEach((child) => flatten(child.id, depth + 1));
    }
  };

  const realRoots: AggregationNode[] = [];
  nodes.forEach((node) => {
    // Identify roots: Modules (level=MODULE=2)
    const isModuleSkeleton =
      !node.isLeaf && !node.aggregationData && node.id.startsWith('LEVEL:2');

    if (
      (!node.isLeaf &&
        node.aggregationData?.id?.level === AggregationLevel.MODULE) ||
      isModuleSkeleton
    ) {
      realRoots.push(node);
    }
  });

  // Sort roots
  realRoots.sort((a, b) => a.id.localeCompare(b.id));

  // Flatten
  realRoots.forEach((r) => {
    flatten(r.id, 0);
  });

  return flatList;
}
