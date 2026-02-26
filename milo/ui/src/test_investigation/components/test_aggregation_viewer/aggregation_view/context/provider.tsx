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

import { OutputTestVerdict } from '@/common/types/verdict';
import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { useTestAggregationContext } from '../../context';
import {
  useAncestryAggregationsQueries,
  useNodeAggregationsQuery,
  useSchemesQuery,
} from '../../hooks';
import { createPrefixForTest, getTestIdentifierPrefixId } from '../../utils';

import { AggregationNode, AggregationViewContext } from './context';
import {
  buildAggregationFilterString,
  flattenDynamicTree,
  mapAggregationToNode,
  NodeChildrenState,
} from './utils';

export interface AggregationViewProviderProps extends PropsWithChildren {
  initialExpandedIds?: string[];
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict;
  autoLocate?: boolean;
}

export function AggregationViewProvider({
  children,
  initialExpandedIds,
  invocation,
  testVariant,
  autoLocate = true,
}: AggregationViewProviderProps) {
  // 1. Consume Shared Filter Context
  const { selectedStatuses, aipFilter } = useTestAggregationContext();

  const aggregationFilterString = useMemo(() => {
    return buildAggregationFilterString(selectedStatuses);
  }, [selectedStatuses]);

  // 2. Data Fetching
  const schemesQuery = useSchemesQuery();

  // Root level modules fetch
  const rootAggsQuery = useNodeAggregationsQuery(
    invocation,
    AggregationLevel.MODULE,
    aggregationFilterString,
    undefined,
    true,
    undefined,
    aipFilter,
  );

  // Deep Linking (Ancestry eager cache warming)
  const selectedTestVariant = testVariant;
  useAncestryAggregationsQueries(
    invocation,
    selectedTestVariant,
    // Note: The filter strings must be passed to cache hit the intermediate nodes,
    // which will be addressed in a follow-up hooks.ts update if they are not already.
  );

  // 3. Dynamic Children State Management
  const [childrenMap, setChildrenMap] = useState<
    Map<string, NodeChildrenState>
  >(new Map());

  const setNodeChildren = useCallback(
    (
      nodeId: string,
      items: AggregationNode[],
      hasNextPage: boolean,
      isFetchingNextPage: boolean,
      fetchNextPage: () => void,
    ) => {
      setChildrenMap((prev) => {
        const next = new Map(prev);
        next.set(nodeId, {
          items,
          hasNextPage,
          isFetchingNextPage,
          fetchNextPage,
        });
        return next;
      });
    },
    [],
  );

  // 4. Tree Construction
  const [expandedIds, setExpandedIds] = useState<Set<string>>(
    new Set(initialExpandedIds),
  );

  // Construct Root Nodes
  const rootNodes = useMemo(() => {
    const schemes = schemesQuery.data?.schemes || {};
    return (rootAggsQuery.data?.aggregations || []).map((agg) =>
      mapAggregationToNode(agg, schemes),
    );
  }, [rootAggsQuery.data?.aggregations, schemesQuery.data?.schemes]);

  // Accumulated search pattern:
  // Automatically fetch the next page if the current page returned fewer than 1000
  // root modules (e.g. mostly passed modules were stripped) but more pages exist.
  useEffect(() => {
    if (
      rootNodes.length > 0 &&
      rootNodes.length < 1000 &&
      rootAggsQuery.hasNextPage &&
      !rootAggsQuery.isFetchingNextPage &&
      !rootAggsQuery.isLoading
    ) {
      rootAggsQuery.fetchNextPage();
    }
  }, [rootNodes.length, rootAggsQuery]);

  // Flatten the dynamic tree
  const flattenedItems = useMemo(() => {
    const flatList = flattenDynamicTree(rootNodes, childrenMap, expandedIds);
    if (rootAggsQuery.hasNextPage) {
      flatList.push({
        id: 'root-load-more',
        label: 'Load more',
        depth: 0,
        isLeaf: true,
        isLoadMore: true,
        hasNextPage: rootAggsQuery.hasNextPage,
        isFetchingNextPage: rootAggsQuery.isFetchingNextPage,
        fetchNextPage: rootAggsQuery.fetchNextPage,
      });
    }
    return flatList;
  }, [
    rootNodes,
    childrenMap,
    expandedIds,
    rootAggsQuery.hasNextPage,
    rootAggsQuery.isFetchingNextPage,
    rootAggsQuery.fetchNextPage,
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
      autoLocate &&
      selectedTestVariant &&
      lastAutoLocatedRef.current !== selectedTestVariant.testId
    ) {
      locateCurrentTest();
      lastAutoLocatedRef.current = selectedTestVariant.testId;
    }
  }, [autoLocate, locateCurrentTest, selectedTestVariant]);

  const isLoading = rootAggsQuery.isLoading;

  return (
    <AggregationViewContext.Provider
      value={{
        expandedIds,
        toggleExpansion,
        flattenedItems,
        invocation,
        setNodeChildren,
        isLoading,
        isError: rootAggsQuery.isError,
        error: rootAggsQuery.error,
        scrollRequest: scrollRequest || undefined,
        locateCurrentTest,
        highlightedNodeId: selectedTestVariant?.testId,
      }}
    >
      {children}
    </AggregationViewContext.Provider>
  );
}
