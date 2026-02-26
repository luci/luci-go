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
import { TestVerdictView } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVerdictPredicate_VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/predicate.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb'; // Added import
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVerdict,
  TestVerdict_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';
import { normalizeDrawerFailureReason } from '@/test_investigation/utils/test_variant_utils';

import { useTestAggregationContext } from '../../context';
import {
  getVariantDefinitionString,
  getVerdictNodeId,
} from '../../context/utils';
import { useTestVerdictsQuery } from '../../hooks';

import {
  TriageStatusGroup,
  TriageViewContext,
  TriageGroup,
  TriageViewNode,
} from './context';

export interface TriageViewProviderProps extends PropsWithChildren {
  initialExpandedIds?: string[];
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict;
  autoLocate?: boolean;
}

const STATUS_ORDER = [
  TestVerdict_Status[TestVerdict_Status.FAILED],
  TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
  TestVerdict_Status[TestVerdict_Status.FLAKY],
  TestVerdict_Status[TestVerdict_Status.SKIPPED],
  TestVerdict_Status[TestVerdict_Status.PRECLUDED],
  TestVerdict_Status[TestVerdict_Status.PASSED],
];

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

export function TriageViewProvider({
  children,
  initialExpandedIds,
  invocation,
  testVariant,
  autoLocate = true,
}: TriageViewProviderProps) {
  // 1. Consume Shared Filter Context
  const { selectedStatuses, aipFilter } = useTestAggregationContext();

  // 2. Data Fetching
  const verdictStatuses = useMemo(() => {
    if (selectedStatuses.size === 0) {
      return [];
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
    TestVerdictView.TEST_VERDICT_VIEW_FULL,
    {
      staleTime: 60 * 1000,
    },
  );

  const lastSyncRef = useRef<{
    loadedCount: number;
    hasNextPage: boolean;
    isFetching: boolean;
  }>({ loadedCount: 0, hasNextPage: false, isFetching: false });

  // Sync Loaded Count & Initial Load Logic
  const loadedCount = verdictsQuery.data?.testVerdicts?.length || 0;
  useEffect(() => {
    const hasNext = verdictsQuery.hasNextPage;
    const isFetching = verdictsQuery.isFetchingNextPage;

    if (
      lastSyncRef.current.loadedCount !== loadedCount ||
      lastSyncRef.current.hasNextPage !== hasNext ||
      lastSyncRef.current.isFetching !== isFetching
    ) {
      lastSyncRef.current = {
        loadedCount,
        hasNextPage: hasNext,
        isFetching,
      };

      // The initial load triggers multiple times as pages come in. As long as we haven't
      // accumulated 1000 items from the current query context, and more pages are available,
      // request the next page. This handles scenarios where sparse optimized pages load with 0 items.
      if (loadedCount < 1000 && hasNext && !isFetching) {
        verdictsQuery.fetchNextPage();
      }
    }
  }, [
    loadedCount,
    verdictsQuery.hasNextPage,
    verdictsQuery.isFetchingNextPage,
    verdictsQuery.fetchNextPage,
    verdictsQuery,
  ]);

  const fetchNextPage = useCallback(() => {
    if (verdictsQuery.hasNextPage && !verdictsQuery.isFetchingNextPage) {
      verdictsQuery.fetchNextPage();
    }
  }, [verdictsQuery]);

  // 3. Merging with Selected Test Variant
  const selectedTestVariant = testVariant;

  const mergedVerdicts = useMemo(() => {
    const fromQuery = verdictsQuery.data?.testVerdicts || [];
    if (!selectedTestVariant) {
      return fromQuery;
    }

    // Check if selected variant is already in query results
    // We match by testId + testIdStructured (moduleVariantHash or variant def)
    const found = fromQuery.find((v) => {
      if (v.testId !== selectedTestVariant.testId) return false;
      const vHash = v.testIdStructured?.moduleVariantHash;
      const sHash = selectedTestVariant.testIdStructured?.moduleVariantHash;
      if (vHash && sHash) {
        return vHash === sHash;
      }
      return (
        getVariantDefinitionString(v.testIdStructured?.moduleVariant?.def) ===
        getVariantDefinitionString(
          selectedTestVariant.testIdStructured?.moduleVariant?.def,
        )
      );
    });

    if (found) {
      return fromQuery;
    }

    // If not found, we append selectedTestVariant as a TestVerdict
    const v: TestVerdict = {
      testId: selectedTestVariant.testId,
      testIdStructured: selectedTestVariant.testIdStructured,
      status: selectedTestVariant.status as unknown as TestVerdict_Status,
      results: selectedTestVariant.results
        ? selectedTestVariant.results.map(
            (b) => b.result as unknown as TestResult,
          )
        : [],
      exonerations: selectedTestVariant.exonerations || [],
      testMetadata: selectedTestVariant.testMetadata,
      isMasked: selectedTestVariant.isMasked,
      statusOverride: selectedTestVariant.statusOverride,
    };
    return [...fromQuery, v];
  }, [verdictsQuery.data, selectedTestVariant]);

  // 4. Grouping
  const statusGroups = useMemo(() => {
    const groupsByStatus = new Map<string, Map<string, TestVerdict[]>>();

    mergedVerdicts.forEach((v) => {
      const statusStr = TestVerdict_Status[v.status];
      if (!groupsByStatus.has(statusStr)) {
        groupsByStatus.set(statusStr, new Map());
      }
      const reasonMap = groupsByStatus.get(statusStr)!;

      // Extract Failure Reasons
      const reasons = new Set<string>();

      if (v.status === TestVerdict_Status.PASSED) {
        reasons.add('Passed');
      } else if (v.status === TestVerdict_Status.SKIPPED) {
        reasons.add('Skipped');
      } else {
        // failed or similar
        let hasFailures = false;
        v.results?.forEach((r) => {
          const status = r.statusV2 || r.status;
          if (
            status === TestResult_Status.FAILED ||
            status === TestResult_Status.EXECUTION_ERRORED
          ) {
            hasFailures = true;
            if (r.failureReason?.primaryErrorMessage) {
              reasons.add(
                normalizeDrawerFailureReason(
                  r.failureReason.primaryErrorMessage,
                ),
              );
            } else {
              reasons.add('No failure reason');
            }
          }
        });

        if (!hasFailures) {
          reasons.add('No failure reason');
        }
      }

      reasons.forEach((reason) => {
        if (!reasonMap.has(reason)) {
          reasonMap.set(reason, []);
        }
        reasonMap.get(reason)!.push(v);
      });
    });

    // Sort and structure
    const result: TriageStatusGroup[] = [];
    STATUS_ORDER.forEach((status) => {
      if (groupsByStatus.has(status)) {
        const reasonMap = groupsByStatus.get(status)!;
        const groups: TriageGroup[] = [];
        reasonMap.forEach((verdicts, reason) => {
          groups.push({ id: `${status}|${reason}`, reason, verdicts });
        });
        // Sort groups by reason text
        groups.sort((a, b) => a.reason.localeCompare(b.reason));

        result.push({
          status,
          count: reasonMap.size,
          groups,
        });
      }
    });
    // Fill in counts
    return result.map((g) => ({
      ...g,
      count: g.groups.reduce((acc, grp) => acc + grp.verdicts.length, 0),
    }));
  }, [mergedVerdicts]);

  // 5. Expansion State
  const [expandedIds, setExpandedIds] = useState<Set<string>>(
    new Set(initialExpandedIds),
  );

  const toggleExpansion = (id: string, expand?: boolean) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      const shouldExpand = expand ?? !next.has(id);
      if (shouldExpand) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  };

  // Auto-expand status nodes when they first load
  const autoExpandedStatusesRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (statusGroups.length > 0) {
      setExpandedIds((prev) => {
        let changed = false;
        const next = new Set(prev);

        statusGroups.forEach((group) => {
          if (!autoExpandedStatusesRef.current.has(group.status)) {
            autoExpandedStatusesRef.current.add(group.status);
            if (!next.has(group.status)) {
              next.add(group.status);
              changed = true;
            }
          }
        });

        return changed ? next : prev;
      });
    }
  }, [statusGroups]);

  // 6. Flattening for Virtualization
  const flattenedItems = useMemo(() => {
    const items: TriageViewNode[] = [];
    STATUS_ORDER.forEach((status) => {
      // Find the group from statusGroups
      const group = statusGroups.find((g) => g.status === status);
      if (!group) return;

      const isStatusExpanded = expandedIds.has(status);
      items.push({
        type: 'status',
        id: status,
        group,
        expanded: isStatusExpanded,
      });

      if (isStatusExpanded) {
        group.groups.forEach((failureGroup) => {
          const isGroupExpanded = expandedIds.has(failureGroup.id);
          items.push({
            type: 'group',
            id: failureGroup.id,
            group: failureGroup,
            expanded: isGroupExpanded,
            parentStatus: status,
          });

          if (isGroupExpanded) {
            failureGroup.verdicts.forEach((v) => {
              items.push({
                type: 'verdict',
                id: `${failureGroup.id}|${getVerdictNodeId(v)}`,
                verdict: v,
                parentGroup: failureGroup.id,
              });
            });
          }
        });
      }
    });
    return items;
  }, [statusGroups, expandedIds]);

  const [scrollRequest, setScrollRequest] = useState<{
    id: string;
    ts: number;
  }>();

  const locateCurrentTest = useCallback(() => {
    // Find the group containing the selected test
    if (!selectedTestVariant) return;

    // Search in statusGroups
    for (const sGroup of statusGroups) {
      for (const group of sGroup.groups) {
        const found = group.verdicts.find((v) => {
          if (v.testId !== selectedTestVariant.testId) return false;
          const vHash = v.testIdStructured?.moduleVariantHash;
          const sHash = selectedTestVariant.testIdStructured?.moduleVariantHash;
          if (vHash && sHash) {
            return vHash === sHash;
          }
          return (
            getVariantDefinitionString(
              v.testIdStructured?.moduleVariant?.def,
            ) ===
            getVariantDefinitionString(
              selectedTestVariant.testIdStructured?.moduleVariant?.def,
            )
          );
        });
        if (found) {
          setExpandedIds((prev) => {
            if (prev.has(sGroup.status) && prev.has(group.id)) return prev;
            const next = new Set(prev);
            next.add(sGroup.status);
            next.add(group.id);
            return next;
          });
          // Update scroll request to trigger a scroll in the virtual tree
          setScrollRequest({
            id: `${group.id}|${getVerdictNodeId(found)}`,
            ts: Date.now(),
          });
          return;
        }
      }
    }
  }, [statusGroups, selectedTestVariant]);

  // Auto locate
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

  return (
    <TriageViewContext.Provider
      value={{
        invocation,
        statusGroups,
        flattenedItems,
        isLoading: verdictsQuery.isLoading,
        isError: verdictsQuery.isError,
        error: verdictsQuery.error,
        expandedIds,
        toggleExpansion,
        locateCurrentTest,
        scrollRequest,
        loadedCount,
        isLoadingMore: verdictsQuery.isFetchingNextPage,
        hasNextPage: verdictsQuery.hasNextPage,
        fetchNextPage,
      }}
    >
      {children}
    </TriageViewContext.Provider>
  );
}
