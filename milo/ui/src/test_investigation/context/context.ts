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

import { createContext, useContext } from 'react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

export interface InvocationContextValue {
  invocation: AnyInvocation;
  rawInvocationId: string;
  project: string | undefined;
  isLegacyInvocation: boolean;
}

export const InvocationContext = createContext<InvocationContextValue | null>(
  null,
);

export interface TestVariantContextValue {
  testVariant: OutputTestVerdict;
  displayStatusString: string;
}
export const TestVariantContext = createContext<TestVariantContextValue | null>(
  null,
);

export function useInvocation() {
  const ctx = useContext(InvocationContext);
  if (!ctx) {
    throw new Error('useInvocation must be used within an InvocationProvider');
  }
  return ctx.invocation;
}

export function useRawInvocationId() {
  const ctx = useContext(InvocationContext);
  if (!ctx) {
    throw new Error(
      'useRawInvocationId must be used within an InvocationProvider',
    );
  }
  return ctx.rawInvocationId;
}

export function useProject() {
  const ctx = useContext(InvocationContext);
  if (!ctx) {
    throw new Error('useProject must be used within a InvocationProvider');
  }
  return ctx.project;
}

export function useIsLegacyInvocation() {
  const ctx = useContext(InvocationContext);
  if (!ctx) {
    throw new Error(
      'useIsLegacyInvocation must be used within an InvocationProvider',
    );
  }
  return ctx.isLegacyInvocation;
}

export function useTestVariant() {
  const ctx = useContext(TestVariantContext);
  if (!ctx) {
    throw new Error('useTestVariant must be used within a TestVariantProvider');
  }
  return ctx.testVariant;
}

export function useDisplayStatusString() {
  const ctx = useContext(TestVariantContext);
  if (!ctx) {
    throw new Error(
      'useDisplayStatusString must be used within a TestVariantProvider',
    );
  }
  return ctx.displayStatusString;
}
