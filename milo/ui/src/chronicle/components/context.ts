// Copyright 2024 The LUCI Authors.
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

import { createContext } from 'react';

import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';

import { WorkflowType } from '../fake_turboci_graph';

export interface ChronicleContextType {
  workplanId: string;
  graph: WorkPlan | undefined;
  valueDataMap: Map<string, ValueData>;

  // Workflow type for fake data generation only.
  workflowType: WorkflowType;
  setWorkflowType: (type: WorkflowType) => void;

  // Selected node ID from URL query param
  selectedNodeId: string | undefined;
  setSelectedNodeId: (id: string | undefined) => void;
}

export const ChronicleContext = createContext<ChronicleContextType>({
  workplanId: '',
  graph: undefined,
  valueDataMap: new Map(),
  workflowType: WorkflowType.ANDROID,
  setWorkflowType: () => {},
  selectedNodeId: undefined,
  setSelectedNodeId: () => {},
});
