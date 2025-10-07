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

import { useQuery } from '@tanstack/react-query';
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  PropsWithChildren,
} from 'react';
import { useNavigate } from 'react-router';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  generateTestInvestigateUrl,
  generateTestInvestigateUrlForLegacyInvocations,
} from '@/common/tools/url_utils';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import {
  useIsLegacyInvocation,
  useRawInvocationId,
} from '@/test_investigation/context/context';
// This import path is now corrected
import {
  buildHierarchyTree,
  buildFailureReasonTree,
  HierarchyBuildResult,
} from '@/test_investigation/utils/drawer_tree_utils';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

import { TestDrawerContext } from './context';

const readMask = [
  'test_id',
  'test_id_structured',
  'variant_hash',
  'variant.def',
  'test_metadata.name',
  'results.*.result.status_v2',
  'results.*.result.failure_reason.primary_error_message',
  'status',
  'results.*.result.expected',
];
const orderBy = 'status_v2_effective';
const resultLimit = 100;
const pageSize = 1000;

export function TestDrawerProvider({ children }: PropsWithChildren) {
  const navigate = useNavigate();

  const invocation = useInvocation();
  const currentTestVariant = useTestVariant();
  const rawInvocationId = useRawInvocationId();
  const resultDbClient = useResultDbClient();
  const isLegacyInvocation = useIsLegacyInvocation();
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const query = useMemo(() => {
    if (isLegacyInvocation) {
      return resultDbClient.QueryTestVariants.query(
        QueryTestVariantsRequest.fromPartial({
          invocations: [invocation.name],
          resultLimit,
          pageSize,
          readMask,
          orderBy,
        }),
      );
    }
    return resultDbClient.QueryTestVariants.query(
      QueryTestVariantsRequest.fromPartial({
        parent: invocation.name,
        resultLimit,
        pageSize,
        readMask,
        orderBy,
      }),
    );
  }, [invocation.name, isLegacyInvocation, resultDbClient.QueryTestVariants]);

  const { data: testVariantsResponse, isPending: isLoadingTestVariants } =
    useQuery<QueryTestVariantsResponse | null, Error, readonly TestVariant[]>({
      ...query,
      enabled: !!invocation.name,
      staleTime: 5 * 60 * 1000,
      select: (data) => data?.testVariants || [],
    });

  const testVariants: readonly TestVariant[] = useMemo(
    () => testVariantsResponse || [],
    [testVariantsResponse],
  );

  const { tree: hierarchyTreeData, idsToExpand: hierarchyIdsToExpand } =
    useMemo((): HierarchyBuildResult => {
      return buildHierarchyTree(testVariants);
    }, [testVariants]);

  const failureReasonTreeData = useMemo(
    () => buildFailureReasonTree(testVariants),
    [testVariants],
  );

  useEffect(() => {
    if (hierarchyIdsToExpand && hierarchyIdsToExpand.length > 0) {
      setExpandedNodes(new Set(hierarchyIdsToExpand));
    }
  }, [hierarchyIdsToExpand]);

  const toggleNodeExpansion = useCallback((nodeId: string) => {
    setExpandedNodes((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(nodeId)) newSet.delete(nodeId);
      else newSet.add(nodeId);
      return newSet;
    });
  }, []);

  const handleDrawerTestSelection = useCallback(
    (selectedTestVariant: TestVariant) => {
      if (
        isRootInvocation(invocation) &&
        selectedTestVariant.testIdStructured
      ) {
        navigate(
          generateTestInvestigateUrl(
            rawInvocationId,
            selectedTestVariant.testIdStructured,
          ),
        );
      } else {
        navigate(
          generateTestInvestigateUrlForLegacyInvocations(
            rawInvocationId,
            selectedTestVariant.testId,
            selectedTestVariant.variantHash,
          ),
        );
      }
    },
    [invocation, navigate, rawInvocationId],
  );

  const contextValue = useMemo(
    () => ({
      isLoading: isLoadingTestVariants,
      hierarchyTree: hierarchyTreeData,
      failureReasonTree: failureReasonTreeData,
      currentTestId: currentTestVariant.testId,
      currentVariantHash: currentTestVariant.variantHash,
      onSelectTestVariant: handleDrawerTestSelection,
      expandedNodes,
      toggleNodeExpansion,
    }),
    [
      isLoadingTestVariants,
      hierarchyTreeData,
      failureReasonTreeData,
      currentTestVariant.testId,
      currentTestVariant.variantHash,
      handleDrawerTestSelection,
      expandedNodes,
      toggleNodeExpansion,
    ],
  );

  return (
    <TestDrawerContext.Provider value={contextValue}>
      {children}
    </TestDrawerContext.Provider>
  );
}
