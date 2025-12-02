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

import { createContext, ReactNode, useMemo, useState } from 'react';
import { useParams } from 'react-router';

import { useQueryNodes } from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';
import { GraphView } from '@/proto/turboci/graph/orchestrator/v1/graph_view.pb';

import { FakeGraphGenerator, WorkflowType } from '../fake_turboci_graph';

const DEMO_WORKPLAN_ID = 'demo';

interface ChronicleContextType {
  workplanId: string;
  graph: GraphView | undefined;

  // Workflow type for fake data generation only.
  workflowType: WorkflowType;
  setWorkflowType: (type: WorkflowType) => void;
}

export const ChronicleContext = createContext<ChronicleContextType>({
  workplanId: '',
  graph: undefined,
  workflowType: WorkflowType.ANDROID,
  setWorkflowType: () => {},
});

export function ChronicleContextProvider({
  children,
}: {
  children: ReactNode;
}) {
  const { workplanId } = useParams<{ workplanId: string }>();
  const [workflowType, setWorkflowType] = useState<WorkflowType>(
    WorkflowType.ANDROID,
  );

  if (!workplanId) {
    throw new Error('Invalid URL: Missing workplanId parameter.');
  }

  const result = useQueryNodes({
    query: [
      {
        select: {
          workplan: {
            inWorkplans: [{ id: workplanId }],
          },
          nodes: [],
          checkPatterns: [],
          stagePatterns: [],
        },
      },
    ],
    typeInfo: { wanted: ['*'], known: ['*'] },
  });

  const queryNodesResponse = result.data;

  const useFakeData = workplanId === DEMO_WORKPLAN_ID;
  const graph = useMemo(() => {
    if (useFakeData) {
      const generator = new FakeGraphGenerator({
        workPlanIdStr: workplanId,
        workflowType: workflowType,
      });
      return generator.generate();
    }

    return queryNodesResponse?.graph?.[workplanId];
  }, [workplanId, workflowType, useFakeData, queryNodesResponse]);

  const value = useMemo(
    () => ({
      workplanId,
      graph,
      workflowType,
      setWorkflowType,
    }),
    [workplanId, graph, workflowType, setWorkflowType],
  );

  return (
    <ChronicleContext.Provider value={value}>
      {children}
    </ChronicleContext.Provider>
  );
}
