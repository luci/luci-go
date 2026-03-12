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

import { ReactNode, useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';

import { useReadWorkPlan } from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { IdentifierKind } from '@/proto/turboci/graph/ids/v1/identifier_kind.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { ValueMask } from '@/proto/turboci/graph/orchestrator/v1/value_mask.pb';

import { FakeGraphGenerator, WorkflowType } from '../fake_turboci_graph';

const DEMO_WORKPLAN_ID = 'demo';

import { ChronicleContext } from './context';

/**
 * Hook to sync node selection with the URL query parameter `nodeId`.
 */
function useNodeSelection() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedNodeId = searchParams.get('nodeId') || undefined;

  const setSelectedNodeId = useCallback(
    (id: string | undefined) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (id) {
            next.set('nodeId', id);
          } else {
            next.delete('nodeId');
          }
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  return { selectedNodeId, setSelectedNodeId };
}

export function ChronicleContextProvider({
  children,
}: {
  children: ReactNode;
}) {
  const { workplanId } = useParams<{ workplanId: string }>();
  const [workflowType, setWorkflowType] = useState<WorkflowType>(
    WorkflowType.ANDROID,
  );
  const { selectedNodeId, setSelectedNodeId } = useNodeSelection();

  if (!workplanId) {
    throw new Error('Invalid URL: Missing workplanId parameter.');
  }

  const useFakeData = workplanId === DEMO_WORKPLAN_ID;

  const result = useReadWorkPlan(
    {
      workplanId: { id: workplanId },
      includedNodeTypes: [
        IdentifierKind.IDENTIFIER_KIND_CHECK,
        IdentifierKind.IDENTIFIER_KIND_CHECK_EDIT,
        IdentifierKind.IDENTIFIER_KIND_STAGE,
        IdentifierKind.IDENTIFIER_KIND_STAGE_ATTEMPT,
        IdentifierKind.IDENTIFIER_KIND_STAGE_EDIT,
      ],
      valueFilter: {
        typeInfo: {
          wanted: { typeUrls: ['*'] },
          known: { typeUrls: [] },
        },
        checkOptions: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkResultData: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkEditOptions: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkEditResultData: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageArgs: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageAttemptDetails: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageAttemptProgressDetails: ValueMask.VALUE_MASK_TYPE,
        stageEditAttemptDetails: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageEditAttemptProgressDetails: ValueMask.VALUE_MASK_TYPE,
      },
    },
    {
      enabled: !useFakeData,
    },
  );

  const workplanValueMap = useMemo(() => {
    if (useFakeData) {
      const generator = new FakeGraphGenerator({
        workPlanIdStr: workplanId,
        workflowType: workflowType,
      });
      return generator.generate();
    }

    const response = result.data;
    const valueDataMap: Map<string, ValueData> = new Map(
      Object.entries(response?.valueData ?? []),
    );

    return {
      workplan: response?.workplan,
      valueDataMap: valueDataMap,
    };
  }, [workplanId, workflowType, useFakeData, result]);

  const value = useMemo(
    () => ({
      workplanId,
      graph: workplanValueMap.workplan,
      valueDataMap: workplanValueMap.valueDataMap,
      workflowType,
      setWorkflowType,
      selectedNodeId,
      setSelectedNodeId,
    }),
    [
      workplanId,
      workplanValueMap.workplan,
      workplanValueMap.valueDataMap,
      workflowType,
      setWorkflowType,
      selectedNodeId,
      setSelectedNodeId,
    ],
  );

  return (
    <ChronicleContext.Provider value={value}>
      {children}
    </ChronicleContext.Provider>
  );
}
