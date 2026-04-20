// Copyright 2024 The LUCI Authors.
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

import { Typography } from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Column,
  MRT_Cell,
  MRT_Row,
} from 'material-react-table';
import { useMemo } from 'react';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { CellWithTooltip } from '@/fleet/components/table/cell_with_tooltip';
import { BotInformation } from '@/fleet/pages/device_details_page/common/bot_information';
import { BotState } from '@/fleet/pages/device_details_page/common/bot_state';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { CHROMEOS_COLUMN_OVERRIDES } from '../../device_list_page/chromeos/chromeos_columns';
import { ChromeOSDevice } from '../../device_list_page/chromeos/chromeos_columns';

interface ChromeOSDeviceDimensionsProps {
  device?: Device;
}

export const ChromeOSDeviceDimensions = ({
  device,
}: ChromeOSDeviceDimensionsProps) => {
  const dimensionRows = useMemo(() => {
    if (!device) return [];

    const customIds = ['dut_id', 'realm', 'state', 'dut_state'];
    const custom = customIds.map((id) => ({
      id,
      value: String(CHROMEOS_COLUMN_OVERRIDES[id]?.accessorFn?.(device) ?? ''),
    }));

    if (!device.deviceSpec) return custom;

    const labelRows = Object.keys(device.deviceSpec.labels)
      .filter((key) => !customIds.includes(key))
      .map((label) => ({
        id: label,
        value: labelValuesToString(device.deviceSpec!.labels[label].values),
      }));

    return [...custom, ...labelRows];
  }, [device]);

  const columns = useMemo<MRT_ColumnDef<{ id: string; value: string }>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'Key',
        size: 200,
      },
      {
        accessorKey: 'value',
        header: 'Value',
        size: 600,
        Cell: ({
          cell,
          row,
          column,
        }: {
          cell: MRT_Cell<{ id: string; value: string }, unknown>;
          row: { original: { id: string; value: string } };
          column: MRT_Column<{ id: string; value: string }, unknown>;
        }): React.ReactNode => {
          const fieldKey = row.original.id;
          const value = cell.getValue() as string;

          const override = CHROMEOS_COLUMN_OVERRIDES[fieldKey];
          if (override?.renderCell && device) {
            // Mock MRT objects for the dimensions view since we are rendering
            // Device properties as rows in a key-value table.
            return override.renderCell({
              cell: {
                getValue: () => value,
              } as unknown as MRT_Cell<ChromeOSDevice, unknown>,
              row: {
                original: device,
              } as unknown as MRT_Row<ChromeOSDevice>,
              column: {
                ...column,
                id: fieldKey,
              } as unknown as MRT_Column<ChromeOSDevice, unknown>,
            });
          }

          return <CellWithTooltip column={column} value={value} />;
        },
      },
    ],
    [device],
  );

  // TODO: Refactor these device dimensions tables to use shared styles.
  const table = useFCDataTable({
    columns,
    data: dimensionRows,
    enablePagination: false,
    enableColumnActions: false,
    enableSorting: false,
    enableColumnFilters: false,
    enableTopToolbar: true,
    enableStickyHeader: true,
    muiTableHeadRowProps: {
      sx: { minHeight: 'unset' },
    },
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        maxHeight: '80vh',
        '--cell-padding-horizontal': '16px',
        '& .Mui-TableHeadCell-Content': {
          minHeight: 'unset !important',
        },
      },
    },
  });

  if (device?.deviceSpec === undefined) {
    return <></>;
  }

  return (
    device && (
      <>
        <BotInformation
          swarmingHost={DEVICE_TASKS_SWARMING_HOST}
          dutId={device?.dutId || ''}
        />

        <BotState
          swarmingHost={DEVICE_TASKS_SWARMING_HOST}
          dutId={device?.dutId || ''}
        />

        <Typography variant="h5" sx={{ mt: 4 }}>
          Device Dimensions
        </Typography>
        <div css={{ marginTop: 16 }}>
          <MaterialReactTable table={table} />
        </div>
      </>
    )
  );
};
