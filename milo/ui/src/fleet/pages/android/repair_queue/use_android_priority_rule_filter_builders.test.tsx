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

import { renderHook } from '@testing-library/react';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilterState } from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useAndroidPriorityRuleFilterBuilders } from './use_android_priority_rule_filter_builders';

const renderFilterBuilders = (workspace: AndroidPageWorkspace = 'Android') =>
  renderHook(() => useAndroidPriorityRuleFilterBuilders(workspace), {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <FakeContextProvider>{children}</FakeContextProvider>
    ),
  });

const mockDimensions = (data: GetDeviceDimensionsResponse) => {
  jest.spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions').mockReturnValue({
    data,
    isLoading: false,
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<
    typeof UseDeviceDimensionsModule.useDeviceDimensions
  >);
};

const optionValues = (
  builder: StringListFilterCategoryBuilder | undefined,
): string[] => (builder?.options ?? []).map((o) => o.value);

describe('useAndroidPriorityRuleFilterBuilders', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('populates core columns and excludes superseded label keys from dynamic labels', () => {
    mockDimensions({
      baseDimensions: {
        id: { values: ['dev-1', 'dev-2'] },
        state: { values: ['MISSING', 'FAILED', 'DYING', 'IDLE'] },
        lab_name: { values: ['mtv', 'sfo46'] },
        host_group: { values: ['group1', 'pte_labs'] },
      },
      labels: {
        state: { values: ['repairing'] },
        run_target: { values: ['seahawk', 'mustang'] },
        host_group: { values: ['group1', 'pte_labs'] },
        lab: { values: ['acs-1265'] },
        id: { values: ['dev-1'] },
        device_id: { values: ['dev-1'] },
        fc_machine_type: { values: ['device'] },
        '1up': { values: ['true'] },
        battery_level: { values: ['100'] },
      },
    });

    const { result } = renderFilterBuilders('Android');
    const builders = result.current.filterBuilders;

    // Core columns
    expect(optionValues(builders.state)).toEqual([
      BLANK_VALUE,
      'MISSING',
      'FAILED',
      'DYING',
      'IDLE',
    ]);
    // Machine type options are static, not read from the dimensions.
    expect(builders.fc_machine_type?.label).toBe('Machine Type');
    expect(optionValues(builders.fc_machine_type)).toEqual([
      BLANK_VALUE,
      'device',
      'host',
    ]);
    expect(optionValues(builders.lab_name)).toEqual([
      BLANK_VALUE,
      'mtv',
      'sfo46',
    ]);
    expect(optionValues(builders.run_target)).toEqual([
      BLANK_VALUE,
      'seahawk',
      'mustang',
    ]);
    expect(optionValues(builders.host_group)).toEqual([BLANK_VALUE, 'group1']);

    // Superseded keys must NOT exist in dynamic labels
    expect(builders['labels."state"']).toBeUndefined();
    expect(builders['labels."run_target"']).toBeUndefined();
    expect(builders['labels."host_group"']).toBeUndefined();
    expect(builders['labels."lab"']).toBeUndefined();
    expect(builders['labels."id"']).toBeUndefined();
    expect(builders['labels."device_id"']).toBeUndefined();
    expect(builders['labels."fc_machine_type"']).toBeUndefined();

    // Non-colliding dynamic labels must exist
    expect(builders['labels."1up"']).toBeDefined();
    expect(builders['labels."battery_level"']).toBeDefined();
  });

  it('correctly evaluates state = "MISSING" without label collision warning', () => {
    mockDimensions({
      baseDimensions: {
        state: { values: ['MISSING', 'FAILED', 'DYING'] },
      },
      labels: {
        state: { values: ['repairing'] },
      },
    });

    const { result } = renderFilterBuilders('Android');
    const builders = result.current.filterBuilders;

    const { result: filterStateResult } = renderHook(
      () => useFilterState(builders, 'state = "MISSING"'),
      {
        wrapper: ({ children }: { children: React.ReactNode }) => (
          <FakeContextProvider>{children}</FakeContextProvider>
        ),
      },
    );

    // There should be zero warnings!
    expect(filterStateResult.current.filterErrors).toEqual([]);
    expect(filterStateResult.current.filterValues?.state).toBeDefined();
  });

  it('omits host_group in Pixel workspace', () => {
    mockDimensions({
      baseDimensions: {
        host_group: { values: ['pte_labs', 'other_group'] },
      },
      labels: {
        run_target: { values: ['pixel_8'] },
      },
    });

    const { result } = renderFilterBuilders('Pixel');
    const builders = result.current.filterBuilders;

    expect(builders.host_group).toBeUndefined();
    expect(builders.run_target).toBeDefined();
  });
});
