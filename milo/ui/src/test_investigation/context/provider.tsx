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
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { InvocationContext, TestVariantContext } from './context';

interface Props {
  children: React.ReactNode;
}

interface InvocationProviderProps extends Props {
  invocation: AnyInvocation;
  rawInvocationId: string;
  project: string | undefined;
  isLegacyInvocation: boolean;
  children: React.ReactNode;
}

export function InvocationProvider({
  invocation,
  rawInvocationId,
  project,
  isLegacyInvocation,
  children,
}: InvocationProviderProps) {
  return (
    <InvocationContext.Provider
      value={{
        invocation,
        rawInvocationId,
        project,
        isLegacyInvocation,
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
