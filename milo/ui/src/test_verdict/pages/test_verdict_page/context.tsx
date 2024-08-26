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
  useState,
} from 'react';

import { Sources } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { OutputTestVerdict } from '@/test_verdict/types';

interface TestVerdictContext {
  readonly invocationID: string;
  readonly project: string;
  readonly testVerdict: OutputTestVerdict;
  readonly setTestVerdict: Dispatch<SetStateAction<OutputTestVerdict>>;
  readonly sources: Sources | null;
}

export const TestVerdictCtx = createContext<TestVerdictContext | null>(null);

export interface TestVerdictProviderProps {
  readonly children: ReactNode;
  readonly invocationID: string;
  readonly project: string;
  readonly testVerdict: OutputTestVerdict;
  readonly sources: Sources | null;
}

export function TestVerdictProvider({
  children,
  invocationID,
  project,
  testVerdict,
  sources,
}: TestVerdictProviderProps) {
  const [verdict, setVerdict] = useState(testVerdict);
  return (
    <TestVerdictCtx.Provider
      value={{
        invocationID,
        project,
        testVerdict: verdict,
        setTestVerdict: setVerdict,
        sources,
      }}
    >
      {children}
    </TestVerdictCtx.Provider>
  );
}
