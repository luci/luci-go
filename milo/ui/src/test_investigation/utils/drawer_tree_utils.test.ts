// Copyright 2026 The LUCI Authors.
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

import { TestAggregation_ModuleStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';

import { getSemanticStatusFromModuleStatus } from './drawer_tree_utils';

describe('drawer_tree_utils', () => {
  it('should have TestAggregation_ModuleStatus defined', () => {
    expect(TestAggregation_ModuleStatus).toBeDefined();
    // Check a value
    expect(TestAggregation_ModuleStatus.SUCCEEDED).toBeDefined();
  });

  it('should map module status correctly', () => {
    expect(
      getSemanticStatusFromModuleStatus(TestAggregation_ModuleStatus.SUCCEEDED),
    ).toBe('success');
    expect(
      getSemanticStatusFromModuleStatus(TestAggregation_ModuleStatus.FAILED),
    ).toBe('failed');
  });
});
