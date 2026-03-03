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

import { createContext, useContext } from 'react';

import { TestAggregation_ModuleStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

export interface TestAggregationContextValue {
  // Filter State
  selectedTestStatuses: Set<VerdictEffectiveStatus>;
  setSelectedTestStatuses: (statuses: Set<VerdictEffectiveStatus>) => void;
  selectedModuleStatuses: Set<TestAggregation_ModuleStatus>;
  setSelectedModuleStatuses: (
    statuses: Set<TestAggregation_ModuleStatus>,
  ) => void;
  aipFilter: string;
  setAipFilter: (filter: string) => void;
}

export const TestAggregationContext =
  createContext<TestAggregationContextValue | null>(null);

export function useTestAggregationContext() {
  const ctx = useContext(TestAggregationContext);
  if (!ctx) {
    throw new Error(
      'useTestAggregationContext must be used within a TestAggregationProvider',
    );
  }
  return ctx;
}
