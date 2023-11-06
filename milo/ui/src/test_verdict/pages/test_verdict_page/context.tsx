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

import { Sources, TestVariant } from '@/common/services/resultdb';

interface TestVariantContext {
  readonly invocationID: string;
  readonly testVariant: TestVariant;
  readonly setTestVariant: Dispatch<SetStateAction<TestVariant>>;
  readonly project: string;
  readonly selectedResultIndex: number;
  readonly setSelectedResultIndex: Dispatch<SetStateAction<number>>;
  readonly sources: Sources;
}

const TestVariantCtx = createContext<TestVariantContext | null>(null);

export interface TestVariantContextProviderProps {
  readonly children: ReactNode;
  readonly invocationID: string;
  readonly variant: TestVariant;
  readonly project: string;
  readonly sources: Sources;
}

export function TestVariantContextProvider({
  children,
  invocationID,
  variant,
  project,
  sources,
}: TestVariantContextProviderProps) {
  const [testVariant, setTestVariant] = useState(variant);
  const [selectedResultIndex, setSelectedResultIndex] = useState(0);
  return (
    <TestVariantCtx.Provider
      value={{
        invocationID,
        testVariant,
        setTestVariant,
        project,
        selectedResultIndex,
        setSelectedResultIndex,
        sources,
      }}
    >
      {children}
    </TestVariantCtx.Provider>
  );
}

export function useInvocationID() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error(
      'useInvocationID can only be used in a TestVariantContextProvider.',
    );
  }
  return context.invocationID;
}

export function useTestVariant() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error(
      'useTestVariant can only be used in a TestVariantContextProvider.',
    );
  }
  return context.testVariant;
}

export function useSetTestVariant() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error(
      'useTestVariant can only be used in a TestVariantContextProvider.',
    );
  }
  return context.setTestVariant;
}

export function useProject() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error('useProject can only be used in a TestVariantContextProvider.');
  }
  return context.project;
}

export function useSelectedResultIndex() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error(
      'useSelectedResultIndex can only be used in a TestVariantContextProvider.',
    );
  }
  return context.selectedResultIndex;
}

export function useSetSelectedResultIndex() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error(
      'useSetSelectedResultIndex can only be used in a TestVariantContextProvider.',
    );
  }
  return context.setSelectedResultIndex;
}

export function useSources() {
  const context = useContext(TestVariantCtx);
  if (!context) {
    throw Error('useSources can only be used in a TestVariantContextProvider.');
  }
  return context.sources;
}
