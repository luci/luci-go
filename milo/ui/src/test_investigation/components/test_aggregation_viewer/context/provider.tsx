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

import { PropsWithChildren, useState } from 'react';

import { TestAggregation_ModuleStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { TestAggregationContext } from './context';

export interface TestAggregationProviderProps extends PropsWithChildren {
  initialAipFilter?: string;
}

const DEFAULT_TEST_STATUSES = new Set<VerdictEffectiveStatus>();
const DEFAULT_MODULE_STATUSES = new Set<TestAggregation_ModuleStatus>();

export function TestAggregationProvider({
  children,
  initialAipFilter,
}: TestAggregationProviderProps) {
  // 1. Status Filter State
  const [selectedTestStatuses, setSelectedTestStatuses] = useState<
    Set<VerdictEffectiveStatus>
  >(DEFAULT_TEST_STATUSES);
  const [selectedModuleStatuses, setSelectedModuleStatuses] = useState<
    Set<TestAggregation_ModuleStatus>
  >(DEFAULT_MODULE_STATUSES);

  // 2. AIP Filter State
  const [aipFilter, setAipFilter] = useState(initialAipFilter || '');

  return (
    <TestAggregationContext.Provider
      value={{
        selectedTestStatuses,
        setSelectedTestStatuses,
        selectedModuleStatuses,
        setSelectedModuleStatuses,
        aipFilter,
        setAipFilter,
      }}
    >
      {children}
    </TestAggregationContext.Provider>
  );
}
