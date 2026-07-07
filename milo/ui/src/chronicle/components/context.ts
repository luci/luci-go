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

import { TurboCIEnvironment } from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';

import { WorkflowType } from '../fake_turboci_graph';

export const DEMO_WORKPLAN_ID = 'demo';
export const DEMO_ENVIRONMENT_NAME = 'demo';

export enum DetectionErrorType {
  Timeout = 'timeout',
  Error = 'error',
  NoAccess = 'no-access',
}

export interface FailedEnvironment {
  env: TurboCIEnvironment;
  errorType: DetectionErrorType;
}

export interface ChronicleContextType {
  workplanId: string;
  graph: WorkPlan | undefined;
  valueDataMap: Map<string, ValueData>;
  activeEnvironment: string | undefined;

  // Workflow type for fake data generation only.
  workflowType: WorkflowType;
  setWorkflowType: (type: WorkflowType) => void;

  // Selected node ID from URL query param
  selectedNodeId: string | undefined;
  setSelectedNodeId: (id: string | undefined) => void;

  // Detection states
  detecting: boolean;
  setDetecting: (detecting: boolean) => void;
  detectionFailed: boolean;
  setDetectionFailed: (failed: boolean) => void;
  detectedEnvironments: TurboCIEnvironment[];
  setActiveEnvironment: (environment: string | undefined) => void;
  failedEnvironments: FailedEnvironment[];
}

export const ChronicleContext = createContext<ChronicleContextType>({
  workplanId: '',
  graph: undefined,
  valueDataMap: new Map(),
  activeEnvironment: undefined,
  workflowType: WorkflowType.ANDROID,
  setWorkflowType: () => {},
  selectedNodeId: undefined,
  setSelectedNodeId: () => {},
  detecting: false,
  setDetecting: () => {},
  detectionFailed: false,
  setDetectionFailed: () => {},
  detectedEnvironments: [],
  setActiveEnvironment: () => {},
  failedEnvironments: [],
});
