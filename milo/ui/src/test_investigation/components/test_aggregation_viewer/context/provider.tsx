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

import { PropsWithChildren, useState } from 'react';

import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { TestAggregationContext } from './context';

export type TestAggregationProviderProps = PropsWithChildren;

const DEFAULT_STATUSES = new Set([
  TestVerdict_Status[TestVerdict_Status.FAILED],
  TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
  TestVerdict_Status[TestVerdict_Status.FLAKY],
]);

export function TestAggregationProvider({
  children,
}: TestAggregationProviderProps) {
  // 1. Status Filter State
  const [selectedStatuses, setSelectedStatuses] =
    useState<Set<string>>(DEFAULT_STATUSES);

  return (
    <TestAggregationContext.Provider
      value={{
        selectedStatuses,
        setSelectedStatuses,
      }}
    >
      {children}
    </TestAggregationContext.Provider>
  );
}
