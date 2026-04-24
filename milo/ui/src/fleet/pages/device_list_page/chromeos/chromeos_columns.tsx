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

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FC_CellProps } from '@/fleet/types/table';

import {
  ChromeOSDevice,
  ChromeOSColumnDef,
  CHROMEOS_COLUMN_OVERRIDES,
} from './chromeos_fields';

export const getChromeOSColumns = (
  columnIds: string[],
): ChromeOSColumnDef[] => {
  const topLevelProtoFields = [
    'id',
    'dut_id',
    'type',
    'state',
    'host',
    'port',
    'realm',
  ];

  return columnIds.map((id) => {
    const isTopLevelProtoField = topLevelProtoFields.includes(id);

    const override = CHROMEOS_COLUMN_OVERRIDES[id] ?? {};

    return {
      accessorKey: id,
      header: id,
      orderByField: 'labels.' + id,
      filterByField: isTopLevelProtoField ? id : `labels."${id}"`,
      enableEditing: false,
      minSize: 70,
      maxSize: 700,
      enableSorting: true,
      accessorFn: (device) => {
        const labels = device.deviceSpec?.labels?.[id]?.values;
        if (!labels) return undefined;

        return labelValuesToString(labels);
      },
      ...(override.renderCell
        ? {
            Cell: (props: FC_CellProps<ChromeOSDevice>) =>
              override.renderCell!(props),
          }
        : {
            Cell: ({ cell }: FC_CellProps<ChromeOSDevice>) => (
              <EllipsisTooltip>{String(cell.getValue() ?? '')}</EllipsisTooltip>
            ),
          }),
      ...(() => {
        const { renderCell: _, ...rest } = override;
        return rest;
      })(),
    };
  });
};
