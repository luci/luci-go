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

import { useMemo } from 'react';

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

// Health percentages computed by RepopulateAndroidRepairTasks and backing the
// "Host Group / Run Target Health" column. They are not device dimensions, so
// they are offered as fixed range filters rather than read from the
// dimensions API. The backend stores them as fractions in [0, 1]; the editor
// presents them as percentages in [0, 100], matching the column.
export const HOST_GROUP_HEALTH_FILTER_KEY = 'host_group_health_pct';
export const RUN_TARGET_HEALTH_FILTER_KEY = 'run_target_health_pct';
export const CLAIMED_BY_FILTER_KEY = 'claimed_by';
export const FC_MACHINE_TYPE_FILTER_KEY = 'fc_machine_type';
const HEALTH_PCT_FILTER_LABELS: Readonly<Record<string, string>> = {
  [HOST_GROUP_HEALTH_FILTER_KEY]: 'Host Group Health',
  [RUN_TARGET_HEALTH_FILTER_KEY]: 'Run Target Health',
};

// Maps Android device base dimensions (android_devices columns) to the
// Android repair task filter map fields they populate
// (infra/fleetconsole/internal/core/android/filter_map.go). Rules are
// evaluated against repair tasks, so only fields of that map are offered.
const ALLOWED_BASE_DIMENSIONS: Record<
  string,
  { filterKey: string; label: string }
> = {
  // The device `id` base dimension is the repair task's `device_id`.
  id: { filterKey: 'device_id', label: 'Device ID' },
  device_id: { filterKey: 'device_id', label: 'Device ID' },
  [FC_MACHINE_TYPE_FILTER_KEY]: {
    filterKey: FC_MACHINE_TYPE_FILTER_KEY,
    label: 'Machine Type',
  },
  lab_name: { filterKey: 'lab_name', label: 'Lab' },
  host_group: { filterKey: 'host_group', label: 'Host Group' },
  run_target: { filterKey: 'run_target', label: 'Run Target' },
  state: { filterKey: 'state', label: 'State' },
  [CLAIMED_BY_FILTER_KEY]: {
    filterKey: CLAIMED_BY_FILTER_KEY,
    label: 'Claimed By',
  },
};

// Device labels that duplicate a first-class repair task column; they are
// offered through that column rather than as `labels."<key>"`.
const SUPERSEDED_LABEL_KEYS: ReadonlySet<string> = new Set([
  'id',
  'device_id',
  FC_MACHINE_TYPE_FILTER_KEY,
  'lab',
  'lab_name',
  'host_group',
  'run_target',
  'state',
  CLAIMED_BY_FILTER_KEY,
  HOST_GROUP_HEALTH_FILTER_KEY,
  RUN_TARGET_HEALTH_FILTER_KEY,
]);

export type AndroidPriorityRuleFilterBuilder =
  | StringListFilterCategoryBuilder
  | RangeFilterCategoryBuilder;

export const useAndroidPriorityRuleFilterBuilders = (
  workspace: AndroidPageWorkspace,
): {
  filterBuilders: Record<string, AndroidPriorityRuleFilterBuilder>;
  isLoading: boolean;
} => {
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.ANDROID });

  const isDimensionsLoaded = Boolean(
    dimensionsQuery.data &&
      dimensionsQuery.data.baseDimensions &&
      dimensionsQuery.data.labels,
  );

  const filterBuilders = useMemo(() => {
    const builders: Record<string, AndroidPriorityRuleFilterBuilder> = {
      device_id: new StringListFilterCategoryBuilder()
        .setLabel('Device ID')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
      // Static: the backend only ever queues `device` and `host` machines.
      [FC_MACHINE_TYPE_FILTER_KEY]: new StringListFilterCategoryBuilder()
        .setLabel('Machine Type')
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          { label: 'device', value: 'device' },
          { label: 'host', value: 'host' },
        ]),
      lab_name: new StringListFilterCategoryBuilder()
        .setLabel('Lab')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
      host_group: new StringListFilterCategoryBuilder()
        .setLabel('Host Group')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
      run_target: new StringListFilterCategoryBuilder()
        .setLabel('Run Target')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
      state: new StringListFilterCategoryBuilder()
        .setLabel('State')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
      [CLAIMED_BY_FILTER_KEY]: new StringListFilterCategoryBuilder()
        .setLabel('Claimed By')
        .setOptions([{ label: BLANK_VALUE, value: BLANK_VALUE }]),
    };

    for (const [filterKey, label] of Object.entries(HEALTH_PCT_FILTER_LABELS)) {
      builders[filterKey] = new RangeFilterCategoryBuilder()
        .setLabel(label)
        .setMin(0)
        .setMax(100)
        .setScaleFactor(0.01)
        .setUnit('%');
    }

    if (isDimensionsLoaded && dimensionsQuery.data) {
      const data = dimensionsQuery.data;

      for (const [key, value] of Object.entries(data.baseDimensions)) {
        const mapped = ALLOWED_BASE_DIMENSIONS[key];
        if (!mapped) continue;
        // Machine type options are fixed; never replace them with
        // whatever subset the device dimensions currently report.
        if (mapped.filterKey === FC_MACHINE_TYPE_FILTER_KEY) continue;
        if (mapped.filterKey === 'host_group' && workspace === 'Pixel')
          continue;

        let values = value.values;
        if (mapped.filterKey === 'host_group' && workspace === 'Android') {
          values = values.filter((v) => v !== 'pte_labs');
        }
        if (values.length === 0) continue;

        builders[mapped.filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(mapped.label)
          .setOptions([
            { label: BLANK_VALUE, value: BLANK_VALUE },
            ...values.map((v) => ({ label: v, value: v })),
          ]);
      }

      // Fall back to labels['run_target'] if baseDimensions['run_target'] is not populated.
      const runTargetValues =
        data.baseDimensions['run_target']?.values ??
        data.labels['run_target']?.values ??
        [];
      if (runTargetValues.length > 0) {
        builders['run_target'] = new StringListFilterCategoryBuilder()
          .setLabel('Run Target')
          .setOptions([
            { label: BLANK_VALUE, value: BLANK_VALUE },
            ...runTargetValues.map((v) => ({ label: v, value: v })),
          ]);
      }

      for (const [key, value] of Object.entries(data.labels)) {
        if (SUPERSEDED_LABEL_KEYS.has(key.toLowerCase())) continue;
        const values = value.values;
        if (values.length === 0) continue;

        const filterKey = `labels."${key}"`;
        builders[filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(key)
          .setOptions([
            { label: BLANK_VALUE, value: BLANK_VALUE },
            ...values.map((v) => ({ label: v, value: v })),
          ]);
      }
    }

    if (workspace === 'Pixel') {
      delete builders['host_group'];
    }

    return builders;
  }, [isDimensionsLoaded, dimensionsQuery.data, workspace]);

  return {
    filterBuilders,
    isLoading: dimensionsQuery.isLoading,
  };
};
