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

import { AssociatedBug } from '@/common/services/luci_analysis';
import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { FormattedCLInfo } from '@/test_investigation/utils/test_info_utils';

export interface TestInfoContextValue {
  formattedCls: FormattedCLInfo[];
  associatedBugs: AssociatedBug[];
  isLoadingAssociatedBugs: boolean;
  testVariantBranch: TestVariantBranch | null | undefined;
}

export const TestInfoContext = createContext<TestInfoContextValue | null>(null);

export function useFormattedCLs() {
  const ctx = useContext(TestInfoContext);
  if (!ctx) {
    throw new Error('useFormattedCLs must be used within a TestInfoProvider');
  }
  return ctx.formattedCls;
}

export function useAssociatedBugs() {
  const ctx = useContext(TestInfoContext);
  if (!ctx) {
    throw new Error('useAssociatedBugs must be used within a TestInfoProvider');
  }
  return ctx.associatedBugs;
}

export function useIsLoadingAssociatedBugs() {
  const ctx = useContext(TestInfoContext);
  if (!ctx) {
    throw new Error(
      'useIsLoadingAssociatedBugs must be used within a TestInfoProvider',
    );
  }

  return ctx.isLoadingAssociatedBugs;
}

export function useTestVariantBranch() {
  const ctx = useContext(TestInfoContext);
  if (!ctx) {
    throw new Error(
      'useTestVariantBranch must be used within a TestInfoProvider',
    );
  }
  return ctx.testVariantBranch;
}
