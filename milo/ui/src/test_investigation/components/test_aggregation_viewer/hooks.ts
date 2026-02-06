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

import { useQueries, useQuery } from '@tanstack/react-query';

import {
  useResultDbClient,
  useSchemaClient,
} from '@/common/hooks/prpc_clients';
import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVerdictPredicate_VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import {
  QueryTestAggregationsRequest,
  QueryTestVerdictsRequest,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { GetSchemaRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/schema.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

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
 * Used to build the skeleton of the aggregation tree.
 *
 * @param invocation - The invocation name (project/invocations/id).
 * @param statuses - List of effective verdict statuses to filter by.
 * @param filter - Optional filter string to refine results (e.g. test name filter).
 * @param pageSize - Number of results to fetch per page.
 * @param options - React Query options.
 */
export function useTestVerdictsQuery(
  invocation: string,
  statuses: TestVerdictPredicate_VerdictEffectiveStatus[],
  filter: string = '',
  pageSize: number = 1000,
  options?: {
    staleTime?: number;
    enabled?: boolean;
  },
) {
  const client = useResultDbClient();
  return useQuery({
    ...client.QueryTestVerdicts.query(
      QueryTestVerdictsRequest.fromPartial({
        parent: invocation,
        predicate: {
          effectiveVerdictStatus: statuses,
          containsTestResultFilter: filter || undefined,
        },
        orderBy: 'ui_priority, test_id_structured',
        pageSize,
      }),
    ),

    enabled: (options?.enabled ?? true) && !!invocation,
    staleTime: options?.staleTime,
  });
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
  invocation: string,
  filter: string,
  enabled: boolean = true,
) {
  const client = useResultDbClient();
  const levels = [
    AggregationLevel.MODULE,
    AggregationLevel.COARSE,
    AggregationLevel.FINE,
  ];

  return useQueries({
    queries: levels.map((level) => ({
      ...client.QueryTestAggregations.query(
        QueryTestAggregationsRequest.fromPartial({
          parent: invocation,
          predicate: {
            aggregationLevel: level,
            filter: filter,
          },
          pageSize: 1000,
        }),
      ),
      enabled: enabled && !!invocation,
      staleTime: 60 * 1000, // 1 minute
    })),
  });
}

/**
 * Fetches test aggregations specific to the ancestry of a selected test variant.
 * Used for Deep Linking to ensure the path to a specific test is fully loaded.
 *
 * @param invocation - The invocation name.
 * @param testVariant - The selected test variant (context) to fetch ancestry for.
 */
export function useAncestryAggregationsQueries(
  invocation: string,
  testVariant: TestVariant | undefined,
) {
  const client = useResultDbClient();
  const testId = testVariant?.testIdStructured;

  const queries = [];

  if (invocation && testId) {
    // 1. Module Aggregation (Exact)
    queries.push({
      ...client.QueryTestAggregations.query(
        QueryTestAggregationsRequest.fromPartial({
          parent: invocation,
          predicate: {
            aggregationLevel: AggregationLevel.MODULE,
            testPrefixFilter: {
              level: AggregationLevel.MODULE,
              id: {
                moduleName: testId.moduleName,
                moduleScheme: testId.moduleScheme,
                moduleVariantHash: testId.moduleVariantHash,
                coarseName: '',
                fineName: '',
                caseName: '',
              },
            },
          },
        }),
      ),
      staleTime: Infinity,
    });

    // 2. Coarse Siblings (Children of Module)
    queries.push({
      ...client.QueryTestAggregations.query(
        QueryTestAggregationsRequest.fromPartial({
          parent: invocation,
          predicate: {
            aggregationLevel: AggregationLevel.COARSE,
            testPrefixFilter: {
              level: AggregationLevel.MODULE,
              id: {
                moduleName: testId.moduleName,
                moduleScheme: testId.moduleScheme,
                moduleVariantHash: testId.moduleVariantHash,
                coarseName: '',
                fineName: '',
                caseName: '',
              },
            },
          },
          pageSize: 1000,
        }),
      ),
      staleTime: 5 * 60 * 1000,
    });

    // 3. Fine Siblings (Children of Coarse)
    if (testId.coarseName) {
      queries.push({
        ...client.QueryTestAggregations.query(
          QueryTestAggregationsRequest.fromPartial({
            parent: invocation,
            predicate: {
              aggregationLevel: AggregationLevel.FINE,
              testPrefixFilter: {
                level: AggregationLevel.COARSE,
                id: {
                  moduleName: testId.moduleName,
                  moduleScheme: testId.moduleScheme,
                  moduleVariantHash: testId.moduleVariantHash,
                  coarseName: testId.coarseName,
                  fineName: '',
                  caseName: '',
                },
              },
            },
            pageSize: 1000,
          }),
        ),
        staleTime: 5 * 60 * 1000,
      });
    }
  }

  return useQueries({ queries });
}
