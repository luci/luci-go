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

import { TestVerdict } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

export interface TriageGroup {
  id: string; // The fully qualified ID, e.g., "FAILED|foo reason"
  reason: string; // Failure Reason (Primary Error Message)
  verdicts: TestVerdict[];
}

export interface TriageStatusGroup {
  status: string; // Enum string
  count: number;
  groups: TriageGroup[];
}

export type TriageViewNode =
  | { type: 'status'; id: string; group: TriageStatusGroup; expanded: boolean }
  | {
      type: 'group';
      id: string;
      group: TriageGroup;
      expanded: boolean;
      parentStatus: string;
    }
  | { type: 'verdict'; id: string; verdict: TestVerdict; parentGroup: string };

export interface TriageViewContextValue {
  // Data
  statusGroups: TriageStatusGroup[];
  flattenedItems: TriageViewNode[];
  isLoading: boolean;

  // State
  expandedIds: Set<string>;
  toggleExpansion: (id: string, expand?: boolean) => void;

  // Interaction
  locateCurrentTest: () => void;
  scrollRequest?: { id: string; ts: number };
}

export const TriageViewContext = createContext<TriageViewContextValue | null>(
  null,
);

export function useTriageViewContext() {
  const ctx = useContext(TriageViewContext);
  if (!ctx) {
    throw new Error(
      'useTriageViewContext must be used within a TriageViewProvider',
    );
  }
  return ctx;
}
