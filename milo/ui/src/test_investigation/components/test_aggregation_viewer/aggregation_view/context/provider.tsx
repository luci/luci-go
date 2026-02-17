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

import {
  PropsWithChildren,
  useCallback,
  useEffect,
  useMemo,
  useState,
  useRef,
} from 'react';

import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVerdictPredicate_VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

import { useDrawerWrapper } from '../../../test_info/context';
import { useTestAggregationContext } from '../../context';
import {
  useAncestryAggregationsQueries,
  useBulkTestAggregationsQueries,
  useSchemesQuery,
  useTestVerdictsQuery,
} from '../../hooks';
import { createPrefixForTest, getTestIdentifierPrefixId } from '../../utils';

import { AggregationViewContext } from './context';
import {
  buildAggregationFilterString,
  buildSkeleton,
  mergeAndFlatten,
  processAggregations,
} from './utils';

export interface AggregationViewProviderProps extends PropsWithChildren {
  initialExpandedIds?: string[];
}

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

export function AggregationViewProvider({
  children,
  initialExpandedIds,
}: AggregationViewProviderProps) {
  const invocation = useInvocation();
  const { isDrawerOpen } = useDrawerWrapper();

  // 1. Consume Shared Filter Context
  // Ensure we are using the correct context hook from shared context
  const {
    selectedStatuses,
    aipFilter,
    loadMoreTrigger,
    setLoadedCount,
    setIsLoadingMore,
  } = useTestAggregationContext();

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
    invocation,
    verdictStatuses,
    aipFilter,
    undefined, // view
    {
      staleTime: 60 * 1000,
    },
  );

  // 3. Deep Linking (Ancestry)
  const selectedTestVariant = useTestVariant();

  // Aggregations (Enrichment)
  const bulkAggregationsQueries = useBulkTestAggregationsQueries(
    invocation,
    aggregationFilterString,
    true,
    aipFilter,
  );

  const ancestryAggregationsQueries = useAncestryAggregationsQueries(
    invocation,
    selectedTestVariant,
  );

  // Combine bulk + ancestry for enrichment
  // Note: Duplicates are handled in processAggregations via Map overwrites.
  const enrichmentQuery = useMemo(
    () => [...bulkAggregationsQueries, ...ancestryAggregationsQueries],
    [bulkAggregationsQueries, ancestryAggregationsQueries],
  );

  // Trigger Load More
  const lastProcessedTrigger = useRef(loadMoreTrigger);
  useEffect(() => {
    if (loadMoreTrigger > lastProcessedTrigger.current) {
      lastProcessedTrigger.current = loadMoreTrigger;
      if (verdictsQuery.hasNextPage && !verdictsQuery.isFetchingNextPage) {
        verdictsQuery.fetchNextPage();
      }
      bulkAggregationsQueries.forEach((q) => {
        if (q.hasNextPage && !q.isFetchingNextPage) {
          q.fetchNextPage();
        }
      });
    }
  }, [loadMoreTrigger, verdictsQuery, bulkAggregationsQueries]);

  // Sync Loaded Count & Initial Load Logic
  const loadedCount = verdictsQuery.data?.testVerdicts?.length || 0;
  useEffect(() => {
    setLoadedCount(loadedCount);

    // Initial Load: Ensure at least 1000 items are loaded if possible
    if (
      loadedCount < 1000 &&
      verdictsQuery.hasNextPage &&
      !verdictsQuery.isFetchingNextPage
    ) {
      verdictsQuery.fetchNextPage();
      bulkAggregationsQueries.forEach((q) => {
        if (q.hasNextPage && !q.isFetchingNextPage) {
          q.fetchNextPage();
        }
      });
    }
  }, [
    loadedCount,
    setLoadedCount,
    verdictsQuery.hasNextPage,
    verdictsQuery.isFetchingNextPage,
    bulkAggregationsQueries,
    verdictsQuery,
  ]);

  // Sync Loading State
  useEffect(() => {
    const isFetching =
      verdictsQuery.isFetchingNextPage ||
      bulkAggregationsQueries.some((q) => q.isFetchingNextPage);
    setIsLoadingMore(isFetching);
  }, [
    verdictsQuery.isFetchingNextPage,
    bulkAggregationsQueries,
    setIsLoadingMore,
  ]);

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

  const [scrollRequest, setScrollRequest] = useState<{
    id: string;
    ts: number;
  }>();

  const locateCurrentTest = useCallback(() => {
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
        if (Array.from(idsToExpand).every((id) => prev.has(id))) return prev;
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
  }, [selectedTestVariant]);

  // Deep Link Auto-Expand
  const lastAutoLocatedRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (
      isDrawerOpen &&
      selectedTestVariant &&
      lastAutoLocatedRef.current !== selectedTestVariant.testId
    ) {
      locateCurrentTest();
      lastAutoLocatedRef.current = selectedTestVariant.testId;
    }
  }, [isDrawerOpen, locateCurrentTest, selectedTestVariant]);

  const isLoading = verdictsQuery.isLoading; // Main skeleton loading

  return (
    <AggregationViewContext.Provider
      value={{
        expandedIds,
        toggleExpansion,
        flattenedItems,
        isLoading,
        isError: verdictsQuery.isError,
        error: verdictsQuery.error,
        scrollRequest,
        locateCurrentTest,
        highlightedNodeId: selectedTestVariant?.testId,
      }}
    >
      {children}
    </AggregationViewContext.Provider>
  );
}
