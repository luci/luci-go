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

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FC_CellProps } from '@/fleet/types/table';

import {
  CHROMEOS_FIELD_DEFINITIONS,
  getLabelValues,
  KnownChromeOSColumnId,
} from './chromeos_fields_config';
import {
  ChromeOSColumnDef,
  ChromeOSDevice,
  FieldDefinition,
} from './chromeos_types';

export type {
  ChromeOSDevice,
  FieldDefinition,
  ChromeOSColumnDef,
  KnownChromeOSColumnId,
};

export const getChromeOSColumns = (
  columnIds: string[],
): ChromeOSColumnDef[] => {
  return columnIds.map((id) => getFieldDefinition(id).columnDef);
};

export type CompleteFieldDefinition = FieldDefinition & {
  id: string;
  header: string;
  accessorFn: (device: ChromeOSDevice) => unknown;
  renderCell?: (props: FC_CellProps<ChromeOSDevice>) => React.ReactNode;
  columnDef: ChromeOSColumnDef;
  filterKey: ChromeOSFilterKey;
};

const createColumnDef = (
  id: string,
  def: FieldDefinition | undefined,
  header: string,
  accessorFn: (device: ChromeOSDevice) => unknown,
  filterKey: ChromeOSFilterKey,
): ChromeOSColumnDef => {
  return {
    id: id,
    header: header,
    orderByField:
      def?.orderByField ?? (def?.type === 'label' ? `labels.${id}` : id),
    filterKey: filterKey,
    enableEditing: false,
    minSize: 70,
    maxSize: 700,
    enableSorting: true,
    accessorFn: accessorFn,
    ...(def?.renderCell
      ? {
          Cell: (props: FC_CellProps<ChromeOSDevice>) => def.renderCell!(props),
        }
      : {
          Cell: ({ cell }: FC_CellProps<ChromeOSDevice>) => {
            const val = cell.getValue();
            const strVal = Array.isArray(val)
              ? labelValuesToString(val as string[])
              : String(val ?? '');
            return <EllipsisTooltip>{strVal}</EllipsisTooltip>;
          },
        }),
  };
};

/**
 * Factory function to get the complete definition for a field.
 * If the field is not in CHROMEOS_FIELD_DEFINITIONS, it generates a fallback
 * definition assuming it is a dynamically discovered label.
 */
export const getFieldDefinition = (
  id: KnownChromeOSColumnId | (string & {}),
): CompleteFieldDefinition => {
  const def = CHROMEOS_FIELD_DEFINITIONS[
    id as keyof typeof CHROMEOS_FIELD_DEFINITIONS
  ] as FieldDefinition | undefined;

  const header = def?.header || id;
  const accessorFn =
    def?.accessorFn || ((device: ChromeOSDevice) => getLabelValues(device, id));
  const filterKey = (def?.filterKey || getFilterKey(id)) as ChromeOSFilterKey;

  const columnDef = createColumnDef(id, def, header, accessorFn, filterKey);

  if (def) {
    return {
      ...def,
      id,
      header,
      accessorFn,
      columnDef,
      filterKey,
    };
  }
  return {
    type: 'label',
    id,
    header,
    accessorFn,
    columnDef,
    filterKey,
  };
};

type ExtractFilterKey<T> = T extends { filterKey: infer K } ? K : never;
export type ConfigFilterKeys = ExtractFilterKey<
  (typeof CHROMEOS_FIELD_DEFINITIONS)[keyof typeof CHROMEOS_FIELD_DEFINITIONS]
>;

export type ChromeOSFilterKey =
  | KnownChromeOSColumnId
  | `labels."${string}"`
  | ConfigFilterKeys;

export const getFilterKey = (
  id: KnownChromeOSColumnId | (string & {}),
): ChromeOSFilterKey => {
  const def = CHROMEOS_FIELD_DEFINITIONS[
    id as keyof typeof CHROMEOS_FIELD_DEFINITIONS
  ] as FieldDefinition | undefined;
  if (def?.filterKey) return def.filterKey;
  if (def?.type === 'base') return id as ChromeOSFilterKey;
  return `labels."${id}"` as ChromeOSFilterKey;
};

export const EXTRA_COLUMN_IDS = [
  'id',
  'dut_id',
  'current_task',
  'realm',
] satisfies (keyof typeof CHROMEOS_FIELD_DEFINITIONS)[];
