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

import React from 'react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

import { InvocationContext, TestVariantContext } from './context';

interface Props {
  children: React.ReactNode;
}

interface InvocationProviderProps extends Props {
  invocation: Invocation;
  rawInvocationId: string;
  project: string | undefined;
  children: React.ReactNode;
}

export function InvocationProvider({
  invocation,
  rawInvocationId,
  project,
  children,
}: InvocationProviderProps) {
  return (
    <InvocationContext.Provider
      value={{
        invocation,
        rawInvocationId,
        project,
      }}
    >
      {children}
    </InvocationContext.Provider>
  );
}

interface TestVariantProviderProps extends Props {
  testVariant: OutputTestVerdict;
  displayStatusString: string;
  children: React.ReactNode;
}

export function TestVariantProvider({
  testVariant,
  displayStatusString,
  children,
}: TestVariantProviderProps) {
  return (
    <TestVariantContext.Provider
      value={{
        testVariant,
        displayStatusString,
      }}
    >
      {children}
    </TestVariantContext.Provider>
  );
}
