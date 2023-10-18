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

import { BugManagement, BugManagementPolicy } from '@/services/project';
import { BugManagementState, PolicyState } from '@/services/rules';

export interface Problem {
  policy: BugManagementPolicy;
  state: PolicyState;
}

// problemsByDescendingActiveAndPriority combines the bug management state with
// configured policies to return details about active and resolved problems.
// Problems are sorted active first, then by descending priority.
export const problemsByDescendingActiveAndPriority = (config: BugManagement | undefined, state: BugManagementState) : Problem[] => {
  if (!state.policyState || !config || !config.policies) {
    return [];
  }
  const result: Problem[] = [];
  state.policyState.forEach((policyState) => {
    if (!policyState.lastActivationTime) {
      // Policy was never active.
      return;
    }
    const policy = config.policies?.find((p) => p.id == policyState.policyId);
    if (!policy) {
      // Policy definition not found; this could be because the config was
      // very recently deleted.
      return;
    }
    result.push({
      policy: policy,
      state: policyState,
    });
  });
  result.sort((a, b) => {
    // The active policy goes first.
    if (a.state.isActive != b.state.isActive) {
      return a.state.isActive ? -1 : 1;
    }
    // Higher priority goes first (e.g. P0 before P1, etc.).
    if (a.policy.priority != b.policy.priority) {
      return a.policy.priority < b.policy.priority ? -1 : 1;
    }
    // Then lower policy ID goes first.
    if (a.policy.id != b.policy.id) {
      return a.policy.id < b.policy.id ? -1 : 1;
    }
    // Problems are the same.
    return 0;
  });
  return result;
};
