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

import { PropsWithChildren, useEffect, useMemo, useState } from 'react';

import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVerdictPredicate_VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

import { useDrawerWrapper } from '../../test_info/context';
import {
  useAncestryAggregationsQueries,
  useBulkTestAggregationsQueries,
  useSchemesQuery,
  useTestVerdictsQuery,
} from '../hooks';
import { createPrefixForTest, getTestIdentifierPrefixId } from '../utils';

import { TestAggregationContext } from './context';
import {
  buildAggregationFilterString,
  buildSkeleton,
  mergeAndFlatten,
  processAggregations,
} from './utils';

export interface TestAggregationProviderProps extends PropsWithChildren {
  initialExpandedIds?: string[];
}

const DEFAULT_STATUSES = new Set([
  TestVerdict_Status[TestVerdict_Status.FAILED],
  TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
  TestVerdict_Status[TestVerdict_Status.FLAKY],
]);

const STATUS_MAP: Record<string, TestVerdictPredicate_VerdictEffectiveStatus> =
  {
    FAILED: TestVerdictPredicate_VerdictEffectiveStatus.FAILED,
    EXECUTION_ERRORED:
      TestVerdictPredicate_VerdictEffectiveStatus.EXECUTION_ERRORED,
    PRECLUDED: TestVerdictPredicate_VerdictEffectiveStatus.PRECLUDED,
    FLAKY: TestVerdictPredicate_VerdictEffectiveStatus.FLAKY,
    SKIPPED: TestVerdictPredicate_VerdictEffectiveStatus.SKIPPED,
    PASSED: TestVerdictPredicate_VerdictEffectiveStatus.PASSED,
    EXONERATED: TestVerdictPredicate_VerdictEffectiveStatus.EXONERATED,
  };

export function TestAggregationProvider({
  children,
  initialExpandedIds,
}: TestAggregationProviderProps) {
  const invocation = useInvocation();
  const invocationName = isRootInvocation(invocation) ? invocation.name : '';
  const { isDrawerOpen } = useDrawerWrapper();

  // 1. Status Filter State
  const [selectedStatuses, setSelectedStatuses] =
    useState<Set<string>>(DEFAULT_STATUSES);

  const aggregationFilterString = useMemo(() => {
    return buildAggregationFilterString(selectedStatuses);
  }, [selectedStatuses]);

  // 2. Data Fetching
  // Schemes (Global, Immutable)
  const schemesQuery = useSchemesQuery();

  // Verdicts (Skeleton)
  const verdictStatuses = useMemo(() => {
    if (selectedStatuses.size === 0) {
      return [
        TestVerdictPredicate_VerdictEffectiveStatus.FAILED,
        TestVerdictPredicate_VerdictEffectiveStatus.EXECUTION_ERRORED,
        TestVerdictPredicate_VerdictEffectiveStatus.FLAKY,
      ];
    }
    const statuses: TestVerdictPredicate_VerdictEffectiveStatus[] = [];
    selectedStatuses.forEach((s) => {
      const mapped = STATUS_MAP[s];
      if (mapped !== undefined) {
        statuses.push(mapped);
      }
    });
    return statuses;
  }, [selectedStatuses]);

  const verdictsQuery = useTestVerdictsQuery(
    invocationName,
    verdictStatuses,
    '',
    1000,
    {
      staleTime: 60 * 1000,
    },
  );

  // 3. Deep Linking (Ancestry)
  const selectedTestVariant = useTestVariant();

  // Aggregations (Enrichment)
  const bulkAggregationsQueries = useBulkTestAggregationsQueries(
    invocationName,
    aggregationFilterString,
  );

  const ancestryAggregationsQueries = useAncestryAggregationsQueries(
    invocationName,
    selectedTestVariant,
  );

  // Combine bulk + ancestry for enrichment
  // Note: Duplicates are handled in processAggregations via Map overwrites.
  const enrichmentQuery = useMemo(
    () => [...bulkAggregationsQueries, ...ancestryAggregationsQueries],
    [bulkAggregationsQueries, ancestryAggregationsQueries],
  );

  // 6. Tree Construction
  const [expandedIds, setExpandedIds] = useState<Set<string>>(
    new Set(initialExpandedIds),
  );

  // Stage 1: Build Skeleton (Stable)
  const { nodes: skeletonNodes, childrenMap: skeletonChildren } =
    useMemo(() => {
      return buildSkeleton(verdictsQuery.data?.testVerdicts);
    }, [verdictsQuery.data?.testVerdicts]);

  // Stage 2: Process Aggregations (Enrichment)
  const { dataMap: aggDataMap } = useMemo(() => {
    return processAggregations(enrichmentQuery);
  }, [enrichmentQuery]);

  // Stage 3: Merge and Flatten (View Generation)
  const flattenedItems = useMemo(() => {
    const schemes = schemesQuery.data?.schemes || {};
    return mergeAndFlatten(
      skeletonNodes,
      skeletonChildren,
      aggDataMap,
      schemes,
      expandedIds,
    );
  }, [
    skeletonNodes,
    skeletonChildren,
    aggDataMap,
    schemesQuery.data,
    expandedIds,
  ]);

  // Expand logic
  const toggleExpansion = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const highlightedNodeId = useMemo(() => {
    if (!selectedTestVariant) {
      return undefined;
    }
    return `${selectedTestVariant.testId}`;
  }, [selectedTestVariant]);

  const [scrollRequest, setScrollRequest] = useState<{
    id: string;
    ts: number;
  }>();

  const locateCurrentTest = () => {
    if (selectedTestVariant && selectedTestVariant.testIdStructured) {
      const struct = selectedTestVariant.testIdStructured;
      const idsToExpand = new Set<string>();

      // Module
      const modulePrefix = createPrefixForTest(
        selectedTestVariant,
        AggregationLevel.MODULE,
      );
      idsToExpand.add(getTestIdentifierPrefixId(modulePrefix));

      if (struct.coarseName) {
        const coarsePrefix = createPrefixForTest(
          selectedTestVariant,
          AggregationLevel.COARSE,
        );
        idsToExpand.add(getTestIdentifierPrefixId(coarsePrefix));
      }
      if (struct.fineName) {
        const finePrefix = createPrefixForTest(
          selectedTestVariant,
          AggregationLevel.FINE,
        );
        idsToExpand.add(getTestIdentifierPrefixId(finePrefix));
      }

      setExpandedIds((prev) => {
        const next = new Set(prev);
        idsToExpand.forEach((id) => next.add(id));
        return next;
      });

      // Trigger Scroll to Leaf
      setScrollRequest({
        id: selectedTestVariant.testId,
        ts: Date.now(),
      });
    }
  };

  // Deep Link Auto-Expand
  useEffect(() => {
    if (isDrawerOpen) {
      locateCurrentTest();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isDrawerOpen, selectedTestVariant?.testId]);

  const isLoading = verdictsQuery.isLoading; // Main skeleton loading

  return (
    <TestAggregationContext.Provider
      value={{
        expandedIds,
        toggleExpansion,
        flattenedItems,
        isLoading,
        highlightedNodeId,
        selectedStatuses,
        setSelectedStatuses,
        scrollRequest,
        locateCurrentTest,
      }}
    >
      {children}
    </TestAggregationContext.Provider>
  );
}
