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

import { createContext, useContext, RefObject } from 'react';

import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { TestNavigationTreeGroup, TestNavigationTreeNode } from '../types';

export interface TestDrawerContextValue {
  isLoading: boolean;
  hierarchyTree: readonly TestNavigationTreeNode[];
  failureReasonTree: readonly TestNavigationTreeGroup[];
  currentTestId: string;
  currentVariantHash: string;
  onSelectTestVariant: (tv: TestVariant) => void;
  expandedNodes: Set<string>;
  toggleNodeExpansion: (nodeId: string) => void;
  selectedItemRef: RefObject<HTMLDivElement | null>;
  isDrawerOpen: boolean;
}

export const TestDrawerContext = createContext<TestDrawerContextValue | null>(
  null,
);

export function useTestDrawer() {
  const context = useContext(TestDrawerContext);
  if (!context) {
    throw new Error('useTestDrawer must be used within a TestDrawerProvider');
  }
  return context;
}
