// Copyright 2023 The LUCI Authors.
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

import { ReactNode, createContext, useContext } from 'react';

import { Cluster } from '@/common/services/luci_analysis';
import { TestResultBundle } from '@/common/services/resultdb';

interface TestResultsContext {
  readonly results: readonly TestResultBundle[];
  readonly clustersMap?: ReadonlyMap<string, readonly Cluster[]>;
}

export const TestResultsCtx = createContext<TestResultsContext | null>(null);

interface TestResultsProviderProps {
  readonly children: ReactNode;
  readonly results: readonly TestResultBundle[];
  readonly clustersMap?: ReadonlyMap<string, readonly Cluster[]>;
}

export function TestResultsProvider({
  children,
  results,
  clustersMap,
}: TestResultsProviderProps) {
  return (
    <TestResultsCtx.Provider
      value={{
        results,
        clustersMap,
      }}
    >
      {children}
    </TestResultsCtx.Provider>
  );
}

export function useResults() {
  const context = useContext(TestResultsCtx);
  if (!context) {
    throw Error('useResults can only be used in a TestResultsProvider.');
  }
  return context.results;
}

export function useClustersByResultId(resultId: string) {
  const context = useContext(TestResultsCtx);
  if (!context) {
    throw Error(
      'useClustersByResultId can only be used in a TestResultsProvider.',
    );
  }
  return context.clustersMap && context.clustersMap.get(resultId);
}
