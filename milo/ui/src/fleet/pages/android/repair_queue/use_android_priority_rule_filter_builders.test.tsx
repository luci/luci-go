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

import {
  RangeFilterCategory,
  RangeFilterCategoryBuilder,
} from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilterState } from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  AndroidPriorityRuleFilterBuilder,
  HOST_GROUP_HEALTH_FILTER_KEY,
  RUN_TARGET_HEALTH_FILTER_KEY,
  useAndroidPriorityRuleFilterBuilders,
} from './use_android_priority_rule_filter_builders';

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
  builder: AndroidPriorityRuleFilterBuilder | undefined,
): string[] =>
  builder instanceof StringListFilterCategoryBuilder
    ? (builder.options ?? []).map((o) => o.value)
    : [];

const renderFilterState = (
  builders: Record<string, AndroidPriorityRuleFilterBuilder>,
  aip160: string,
) =>
  renderHook(() => useFilterState(builders, aip160), {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <FakeContextProvider>{children}</FakeContextProvider>
    ),
  });

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

  describe('health percentage filters', () => {
    const HEALTH_KEYS = [
      HOST_GROUP_HEALTH_FILTER_KEY,
      RUN_TARGET_HEALTH_FILTER_KEY,
    ];

    it.each<AndroidPageWorkspace>(['Android', 'Pixel'])(
      'offers host group and run target health as 0-100%% ranges in %s',
      (workspace) => {
        mockDimensions({
          baseDimensions: {},
          // A device label with the same name must not shadow the column.
          labels: { host_group_health_pct: { values: ['0.5'] } },
        });

        const { result } = renderFilterBuilders(workspace);
        const builders = result.current.filterBuilders;

        expect(builders['labels."host_group_health_pct"']).toBeUndefined();
        for (const key of HEALTH_KEYS) {
          const builder = builders[key];
          expect(builder).toBeInstanceOf(RangeFilterCategoryBuilder);
          if (!(builder instanceof RangeFilterCategoryBuilder)) continue;
          expect(builder.min).toBe(0);
          expect(builder.max).toBe(100);
          expect(builder.scaleFactor).toBe(0.01);
        }
        expect(builders.host_group_health_pct?.label).toBe('Host Group Health');
        expect(builders.run_target_health_pct?.label).toBe('Run Target Health');
      },
    );

    it('offers health filters before the device dimensions load', () => {
      jest
        .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
        .mockReturnValue({
          data: undefined,
          isLoading: true,
          isPending: true,
          isError: false,
          error: null,
        } as unknown as ReturnType<
          typeof UseDeviceDimensionsModule.useDeviceDimensions
        >);

      const { result } = renderFilterBuilders('Android');

      expect(result.current.filterBuilders.host_group_health_pct).toBeDefined();
      expect(result.current.filterBuilders.run_target_health_pct).toBeDefined();
    });

    it('parses stored fractional rules as percentages without warnings', () => {
      mockDimensions({ baseDimensions: {}, labels: {} });
      const { result } = renderFilterBuilders('Android');

      const { result: filterState } = renderFilterState(
        result.current.filterBuilders,
        'host_group_health_pct <= 0.5 AND run_target_health_pct >= 0.2 AND run_target_health_pct <= 0.8',
      );

      expect(filterState.current.filterErrors).toEqual([]);
      const hostGroup = filterState.current.filterValues?.host_group_health_pct;
      const runTarget = filterState.current.filterValues?.run_target_health_pct;
      expect(hostGroup).toBeInstanceOf(RangeFilterCategory);
      expect(runTarget).toBeInstanceOf(RangeFilterCategory);
      if (
        !(hostGroup instanceof RangeFilterCategory) ||
        !(runTarget instanceof RangeFilterCategory)
      ) {
        return;
      }
      expect(hostGroup.value).toEqual({ max: 50 });
      expect(runTarget.value).toEqual({ min: 20, max: 80 });
      expect(hostGroup.getChipLabel()).toBe('[ Host Group Health ]: to 50');
    });

    it('serializes percentages back to the fractions the backend stores', () => {
      mockDimensions({ baseDimensions: {}, labels: {} });
      const { result } = renderFilterBuilders('Android');

      const { result: filterState } = renderFilterState(
        result.current.filterBuilders,
        'run_target_health_pct >= 0.25',
      );
      const runTarget = filterState.current.filterValues?.run_target_health_pct;
      expect(runTarget).toBeInstanceOf(RangeFilterCategory);
      if (!(runTarget instanceof RangeFilterCategory)) return;

      runTarget.value = { min: 10, max: 75 };
      expect(runTarget.toAIP160()).toBe(
        'run_target_health_pct >= 0.1 AND run_target_health_pct <= 0.75',
      );
    });
  });
});
