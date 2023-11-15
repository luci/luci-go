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

import {
  Dispatch,
  ReactNode,
  SetStateAction,
  createContext,
  useContext,
  useState,
} from 'react';

import { TestResultBundle } from '@/common/services/resultdb';

interface TestResultsContext {
  readonly results: readonly TestResultBundle[];
  readonly selectedResultIndex: number;
  readonly setSelectedResultIndex: Dispatch<SetStateAction<number>>;
}

export const TestResultsCtx = createContext<TestResultsContext | null>(null);

interface TestResultsContextProviderProps {
  readonly children: ReactNode;
  readonly results: readonly TestResultBundle[];
}

export function TestResultsContextProvider({
  children,
  results,
}: TestResultsContextProviderProps) {
  const [selectedResultIndex, setSelectedResultIndex] = useState(0);
  return (
    <TestResultsCtx.Provider
      value={{
        results,
        selectedResultIndex,
        setSelectedResultIndex,
      }}
    >
      {children}
    </TestResultsCtx.Provider>
  );
}

export function useResults() {
  const context = useContext(TestResultsCtx);
  if (!context) {
    throw Error(
      'useSelectedResultIndex can only be used in a TestResultsContextProvider.',
    );
  }
  return context.results;
}

export function useSelectedResultIndex() {
  const context = useContext(TestResultsCtx);
  if (!context) {
    throw Error(
      'useSelectedResultIndex can only be used in a TestResultsContextProvider.',
    );
  }
  return context.selectedResultIndex;
}

export function useSetSelectedResultIndex() {
  const context = useContext(TestResultsCtx);
  if (!context) {
    throw Error(
      'useSetSelectedResultIndex can only be used in a TestResultsContextProvider.',
    );
  }
  return context.setSelectedResultIndex;
}
