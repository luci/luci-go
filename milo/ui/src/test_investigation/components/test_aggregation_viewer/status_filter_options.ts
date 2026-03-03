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
import { VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

export const TEST_PREFIX = 'TEST:';
export const MOD_PREFIX = 'MOD:';

export interface FilterOption<T> {
  id: string; // Used as the underlying string value for the Select component
  enumValue: T;
  label: string;
}

export const TEST_STATUS_OPTIONS: FilterOption<VerdictEffectiveStatus>[] = [
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_FAILED}`,
    enumValue: VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_FAILED,
    label: 'Failed',
  },
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED}`,
    enumValue:
      VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED,
    label: 'Execution Errored',
  },
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_FLAKY}`,
    enumValue: VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_FLAKY,
    label: 'Flaky',
  },
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_PASSED}`,
    enumValue: VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_PASSED,
    label: 'Passed',
  },
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_SKIPPED}`,
    enumValue: VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_SKIPPED,
    label: 'Skipped',
  },
  {
    id: `${TEST_PREFIX}${VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_PRECLUDED}`,
    enumValue: VerdictEffectiveStatus.VERDICT_EFFECTIVE_STATUS_PRECLUDED,
    label: 'Precluded',
  },
];

export const MODULE_STATUS_OPTIONS: FilterOption<TestAggregation_ModuleStatus>[] =
  [
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.SUCCEEDED}`,
      enumValue: TestAggregation_ModuleStatus.SUCCEEDED,
      label: 'Succeeded',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.FLAKY}`,
      enumValue: TestAggregation_ModuleStatus.FLAKY,
      label: 'Flaky',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.FAILED}`,
      enumValue: TestAggregation_ModuleStatus.FAILED,
      label: 'Failed',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.SKIPPED}`,
      enumValue: TestAggregation_ModuleStatus.SKIPPED,
      label: 'Skipped',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.CANCELLED}`,
      enumValue: TestAggregation_ModuleStatus.CANCELLED,
      label: 'Cancelled',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.RUNNING}`,
      enumValue: TestAggregation_ModuleStatus.RUNNING,
      label: 'Running',
    },
    {
      id: `${MOD_PREFIX}${TestAggregation_ModuleStatus.PENDING}`,
      enumValue: TestAggregation_ModuleStatus.PENDING,
      label: 'Pending',
    },
  ];

export const ALL_OPTIONS_MAP = new Map<string, FilterOption<unknown>>([
  ...TEST_STATUS_OPTIONS.map((opt) => [opt.id, opt] as const),
  ...MODULE_STATUS_OPTIONS.map((opt) => [opt.id, opt] as const),
]);

export function formatSelectedStatuses(selectedIds: string[]): string {
  const numTestOptions = TEST_STATUS_OPTIONS.length;
  const numModOptions = MODULE_STATUS_OPTIONS.length;
  const totalOptions = numTestOptions + numModOptions;

  if (selectedIds.length === 0 || selectedIds.length === totalOptions) {
    return 'All';
  }

  return selectedIds
    .map((id) => {
      const option = ALL_OPTIONS_MAP.get(id);
      if (!option) {
        return id;
      }
      if (id.startsWith(MOD_PREFIX)) {
        return `Mod: ${option.label}`;
      }
      return option.label;
    })
    .join(', ');
}
