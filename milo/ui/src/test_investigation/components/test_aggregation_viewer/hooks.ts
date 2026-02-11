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

import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';

import {
  useResultDbClient,
  useSchemaClient,
} from '@/common/hooks/prpc_clients';
import {
  AggregationLevel,
  TestVerdictView,
  TestIdentifierPrefix,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVerdictPredicate_VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import {
  QueryTestAggregationsRequest,
  QueryTestVerdictsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { RootInvocation_FinalizationState } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { GetSchemaRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/schema.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  AnyInvocation,
  isLegacyInvocation,
  isRootInvocation,
} from '@/test_investigation/utils/invocation_utils';

/**
 * Fetches the test scheme definitions from the schema service.
 * Used to interpret module schemes in test IDs.
 */
export function useSchemesQuery() {
  const client = useSchemaClient();
  return useQuery({
    ...client.Get.query(GetSchemaRequest.fromPartial({ name: 'schema' })),
    staleTime: Infinity,
  });
}

/**
 * Fetches test verdicts for a given invocation, filtered by status and test result filter.
 * Accumulates results across pages until the requested pageSize is reached.
 *
 * @param invocation - The invocation name (project/invocations/id).
 * @param statuses - List of effective verdict statuses to filter by.
 * @param filter - Optional filter string to refine results (e.g. test name filter).
 * @param pageSize - Number of results to fetch per page / total target limit.
 * @param options - React Query options.
 */
export function useTestVerdictsQuery(
  invocation: AnyInvocation | null | undefined,
  statuses: TestVerdictPredicate_VerdictEffectiveStatus[],
  filter: string = '',
  view: TestVerdictView = TestVerdictView.TEST_VERDICT_VIEW_UNSPECIFIED,
  pageSize: number = 1000,
  options?: {
    staleTime?: number;
    enabled?: boolean;
  },
) {
  const invocationName = isRootInvocation(invocation) ? invocation.name : '';
  const isFinalized = isRootInvocation(invocation)
    ? invocation.finalizationState ===
      RootInvocation_FinalizationState.FINALIZED
    : isLegacyInvocation(invocation)
      ? invocation.state === Invocation_State.FINALIZED
      : false;

  const staleTime = isFinalized ? Infinity : (options?.staleTime ?? 60 * 1000);

  const client = useResultDbClient();
  const query = useInfiniteQuery({
    ...client.QueryTestVerdicts.queryPaged(
      QueryTestVerdictsRequest.fromPartial({
        parent: invocationName,
        predicate: {
          effectiveVerdictStatus: statuses,
          containsTestResultFilter: filter || undefined,
        },
        view,
        orderBy: 'ui_priority, test_id_structured',
        pageSize,
      }),
    ),
    enabled: (options?.enabled ?? true) && !!invocationName,
    staleTime,
  });

  const allVerdicts = useMemo(() => {
    return query.data?.pages.flatMap((p) => p.testVerdicts || []) || [];
  }, [query.data]);

  useEffect(() => {
    if (
      query.hasNextPage &&
      !query.isFetching &&
      allVerdicts.length < pageSize
    ) {
      query.fetchNextPage();
    }
  }, [
    query.hasNextPage,
    query.isFetching,
    allVerdicts.length,
    pageSize,
    query.fetchNextPage,
    query,
  ]);

  return {
    data: query.data ? { testVerdicts: allVerdicts } : undefined,
    isLoading:
      query.isLoading || (query.hasNextPage && allVerdicts.length < pageSize),
  };
}

/**
 * Helper to fetch a single test aggregation level using infinite query and accumulating
 * up to a target size limit to account for empty pages during optimization.
 */
function useAccumulatedAggregationsQuery(
  invocation: AnyInvocation | null | undefined,
  level: AggregationLevel,
  filter?: string,
  testPrefixFilter?: TestIdentifierPrefix,
  enabled: boolean = true,
  staleTimeOverride?: number,
  pageSize: number = 1000,
) {
  const invocationName = isRootInvocation(invocation) ? invocation.name : '';
  const isFinalized = isRootInvocation(invocation)
    ? invocation.finalizationState ===
      RootInvocation_FinalizationState.FINALIZED
    : isLegacyInvocation(invocation)
      ? invocation.state === Invocation_State.FINALIZED
      : false;

  const staleTime = isFinalized ? Infinity : (staleTimeOverride ?? 60 * 1000);

  const client = useResultDbClient();
  const query = useInfiniteQuery({
    ...client.QueryTestAggregations.queryPaged(
      QueryTestAggregationsRequest.fromPartial({
        parent: invocationName,
        predicate: {
          aggregationLevel: level,
          filter: filter || undefined,
          testPrefixFilter: testPrefixFilter || undefined,
        },
        pageSize,
      }),
    ),
    enabled: enabled && !!invocationName,
    staleTime,
  });

  const allAggregations = useMemo(() => {
    return query.data?.pages.flatMap((p) => p.aggregations || []) || [];
  }, [query.data]);

  useEffect(() => {
    if (
      query.hasNextPage &&
      !query.isFetching &&
      allAggregations.length < pageSize
    ) {
      query.fetchNextPage();
    }
  }, [
    query.hasNextPage,
    query.isFetching,
    allAggregations.length,
    pageSize,
    query.fetchNextPage,
    query,
  ]);

  return {
    data: query.data ? { aggregations: allAggregations } : undefined,
    // Provide a consistent interface matching useQueries output array elements
    isLoading:
      query.isLoading ||
      (query.hasNextPage && allAggregations.length < pageSize),
  };
}

/**
 * Fetches test aggregations for Module, Coarse, and Fine levels in parallel.
 * This "Bulk" strategy provides a broad overview of the test results structure.
 *
 * @param invocation - The invocation name.
 * @param filter - Filter string to apply to the aggregations.
 * @param enabled - Whether the queries should be enabled.
 */
export function useBulkTestAggregationsQueries(
  invocation: AnyInvocation | null | undefined,
  filter: string,
  enabled: boolean = true,
) {
  const modSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.MODULE,
    filter,
    undefined,
    enabled,
  );
  const crsSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.COARSE,
    filter,
    undefined,
    enabled,
  );
  const fneSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.FINE,
    filter,
    undefined,
    enabled,
  );

  return [modSq, crsSq, fneSq];
}

/**
 * Fetches test aggregations specific to the ancestry of a selected test variant.
 * Used for Deep Linking to ensure the path to a specific test is fully loaded.
 *
 * @param invocation - The invocation object.
 * @param testVariant - The selected test variant (context) to fetch ancestry for.
 */
export function useAncestryAggregationsQueries(
  invocation: AnyInvocation | null | undefined,
  testVariant: TestVariant | undefined,
) {
  const testId = testVariant?.testIdStructured;

  // 1. Module Aggregation (Exact)
  const modFilter = testId
    ? {
        level: AggregationLevel.MODULE,
        id: {
          moduleName: testId.moduleName,
          moduleScheme: testId.moduleScheme,
          moduleVariant: testVariant?.variant,
          moduleVariantHash: testId.moduleVariantHash,
          coarseName: '',
          fineName: '',
          caseName: '',
        },
      }
    : undefined;

  const modSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.MODULE,
    undefined,
    modFilter,
    !!(invocation && testId),
    Infinity,
  );

  // 2. Coarse Siblings (Children of Module)
  const crsFilter = testId
    ? {
        level: AggregationLevel.MODULE,
        id: {
          moduleName: testId.moduleName,
          moduleScheme: testId.moduleScheme,
          moduleVariant: testVariant?.variant || undefined,
          moduleVariantHash: testId.moduleVariantHash,
          coarseName: '',
          fineName: '',
          caseName: '',
        },
      }
    : undefined;

  const crsSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.COARSE,
    undefined,
    crsFilter,
    !!(invocation && testId),
    5 * 60 * 1000,
  );

  // 3. Fine Siblings (Children of Coarse)
  const fneFilter =
    testId && testId.coarseName
      ? {
          level: AggregationLevel.COARSE,
          id: {
            moduleName: testId.moduleName,
            moduleScheme: testId.moduleScheme,
            moduleVariant: testVariant?.variant || undefined,
            moduleVariantHash: testId.moduleVariantHash,
            coarseName: testId.coarseName,
            fineName: '',
            caseName: '',
          },
        }
      : undefined;

  const fneSq = useAccumulatedAggregationsQuery(
    invocation,
    AggregationLevel.FINE,
    undefined,
    fneFilter,
    !!(invocation && testId && testId.coarseName),
    5 * 60 * 1000,
  );

  const results = [];
  if (invocation && testId) {
    results.push(modSq);
    results.push(crsSq);
    if (testId.coarseName) {
      results.push(fneSq);
    }
  }

  return results;
}
