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

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';

export type RepairQueueFilterKey = string;

/**
 * The values `types.PeripheralState` can hold. Hardcoded rather than read from
 * the device dimension API because the repair_tasks columns store normalized
 * values while the API returns the raw UFS strings they were derived from.
 *
 * `UNKNOWN` is deliberately absent: the UI renders it for
 * PERIPHERAL_STATE_UNSPECIFIED, but the columns default to NOT_APPLICABLE and
 * never store unspecified, so a rule on it would score 0 forever.
 */
const PERIPHERAL_STATE_OPTIONS: readonly string[] = [
  'OK',
  'BROKEN',
  'MISSING',
  'NOT_APPLICABLE',
];

interface CoreRepairColumnConfig {
  readonly filterKey: string;
  readonly header: string;
  /**
   * The device dimension key to source the option list from. This is the same
   * key `RepopulateChromeOSRepairTasks` reads when it fills the column, so the
   * options offered here are exactly the values a rule can match.
   */
  readonly dimensionSourceKey?: string;
  /**
   * A fixed option list, for columns Fleet Console computes itself and which
   * therefore have no matching device dimension.
   */
  readonly staticOptions?: readonly string[];
  /**
   * Label keys this column carries the same data as. They are dropped from the
   * dynamic label list below so the dropdown does not offer the same filter
   * twice. Where a label and a column disagree, the column wins: it is indexed
   * and the jsonb path is a containment scan.
   */
  readonly supersededLabelKeys: readonly string[];
}

const CORE_REPAIR_COLUMNS: readonly CoreRepairColumnConfig[] = [
  {
    filterKey: 'dut_id',
    header: 'Dut ID',
    dimensionSourceKey: 'dut_id',
    // `dut_name` holds the same hostname under a different key.
    supersededLabelKeys: ['dut_id', 'dut_name'],
  },
  {
    filterKey: 'pools',
    header: 'Pool',
    dimensionSourceKey: 'label-pool',
    supersededLabelKeys: ['label-pool', 'pool', 'pools'],
  },
  {
    filterKey: 'model',
    header: 'Model',
    dimensionSourceKey: 'label-model',
    supersededLabelKeys: ['label-model', 'model'],
  },
  {
    filterKey: 'board',
    header: 'Board',
    dimensionSourceKey: 'label-board',
    supersededLabelKeys: ['label-board', 'board'],
  },
  {
    filterKey: 'dut_state',
    header: 'State',
    dimensionSourceKey: 'dut_state',
    supersededLabelKeys: ['dut_state'],
  },
  // The three peripheral columns hold normalized states, the matching labels
  // hold the raw UFS values. `servo_state = "BROKEN"` catches every failure
  // mode at once, `labels."label-servo_state" = "SERVOD_ISSUE"` targets one, so
  // both are worth offering. The headers here stay distinct from the label
  // headers ("Servo State", "Bluetooth State") to keep them apart in the list.
  {
    filterKey: 'servo_state',
    header: 'Servo',
    staticOptions: PERIPHERAL_STATE_OPTIONS,
    supersededLabelKeys: [],
  },
  {
    filterKey: 'wifi_state',
    header: 'Wi-Fi',
    staticOptions: PERIPHERAL_STATE_OPTIONS,
    supersededLabelKeys: [],
  },
  {
    filterKey: 'bluetooth_state',
    header: 'Bluetooth',
    staticOptions: PERIPHERAL_STATE_OPTIONS,
    supersededLabelKeys: [],
  },
];

export const useRepairQueueFilterBuilders = (): {
  filterBuilders: Record<string, StringListFilterCategoryBuilder>;
  isLoading: boolean;
} => {
  const { availableFields, getValues, isLoading } = useChromeOSFields();

  const filterBuilders = useMemo(() => {
    const filters: Record<string, StringListFilterCategoryBuilder> = {};

    const build = (label: string, values: readonly string[]) =>
      new StringListFilterCategoryBuilder()
        .setLabel(label)
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...values.map((v) => ({ label: v, value: v })),
        ]);

    // 1. Core repair columns available on the repair_tasks table
    for (const config of CORE_REPAIR_COLUMNS) {
      const values = config.dimensionSourceKey
        ? getValues(config.dimensionSourceKey)
        : (config.staticOptions ?? []);

      filters[config.filterKey] = build(config.header, values);
    }

    const supersededLabelKeys = new Set(
      CORE_REPAIR_COLUMNS.flatMap((c) =>
        c.supersededLabelKeys.map((k) => k.toLowerCase()),
      ),
    );

    // 2. Include dynamic device labels from labels.* JSONB column.
    // Exclude device inventory base fields (e.g. type, id, state, realm) not on repair_tasks.
    availableFields.forEach((def) => {
      if (def.type !== 'label') {
        return;
      }
      if (
        filters[def.filterKey] ||
        supersededLabelKeys.has(def.id.toLowerCase())
      ) {
        return;
      }
      const values = getValues(def.id);
      if (values.length === 0) {
        return;
      }

      filters[def.filterKey] = build(def.header, values);
    });

    return filters;
  }, [availableFields, getValues]);

  return {
    filterBuilders,
    isLoading,
  };
};
