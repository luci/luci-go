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
// limitations under the License.

import { Typography } from '@mui/material';
import {
  MRT_Cell,
  MRT_Column,
  MRT_Row,
  MRT_TableInstance,
  MaterialReactTable,
} from 'material-react-table';
import { ReactNode, useMemo } from 'react';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { FC_CellProps } from '@/fleet/types/table';
import { orderMRTColumns } from '@/fleet/utils/columns';
import {
  BrowserDevice,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getBrowserColumn } from '../../device_list_page/browser/browser_columns';
import { BROWSER_COLUMN_OVERRIDES } from '../../device_list_page/browser/browser_fields';
import { BotInformation } from '../common/bot_information';
import { BotState } from '../common/bot_state';

export interface BrowserDeviceDimensionsProps {
  device?: BrowserDevice;
  botId?: string;
  swarmingHost?: string;
}

export interface DeviceDimension {
  id: string;
  key: string;
  value: unknown;
}

export const BrowserDeviceDimensions = ({
  device,
  botId,
  swarmingHost,
}: BrowserDeviceDimensionsProps) => {
  const rows = useMemo<DeviceDimension[]>(() => {
    if (device?.id === undefined) {
      return [];
    }
    const ids = [
      'id',
      'realm',
      ...Object.keys(device.swarmingLabels || {}).map(
        (l) => `${BROWSER_SWARMING_SOURCE}.${l}`,
      ),
      ...Object.keys(device.ufsLabels || {}).map(
        (l) => `${BROWSER_UFS_SOURCE}.${l}`,
      ),
    ];

    let columnsRecord = ids.map((id) => getBrowserColumn(id));
    columnsRecord = orderMRTColumns(Platform.CHROMIUM, columnsRecord);

    return columnsRecord.map((col) => {
      return {
        key: (col.header as string) ?? col.accessorKey,
        value: col.accessorFn?.(device as BrowserDevice) ?? '',
        // This field is unique and will be used
        // to find custom renderCell functions.
        id: col.accessorKey as string,
      };
    });
  }, [device]);

  const columns = useMemo(() => {
    return [
      { accessorKey: 'key', header: 'Key', size: 150 },
      {
        accessorKey: 'value',
        header: 'Value',
        size: 350,
        Cell: (params: {
          cell: MRT_Cell<DeviceDimension, unknown>;
          row: MRT_Row<DeviceDimension>;
          table: MRT_TableInstance<DeviceDimension>;
          column: MRT_Column<DeviceDimension, unknown>;
        }) => {
          const id = params.row.original.id;
          const override = BROWSER_COLUMN_OVERRIDES[id as string];

          if (override && override.Cell) {
            // Need to map our simplified MRT row into proper payload types.
            const cellProps = {
              cell: params.cell as unknown as FC_CellProps<BrowserDevice>['cell'],
              row: {
                ...params.row,
                original: device as BrowserDevice,
              } as unknown as FC_CellProps<BrowserDevice>['row'],
              column:
                params.column as unknown as FC_CellProps<BrowserDevice>['column'],
            };
            return (
              override.Cell as (
                params: FC_CellProps<BrowserDevice>,
              ) => ReactNode
            )(cellProps);
          }

          return (
            <EllipsisTooltip>
              {params.cell.getValue() as string}
            </EllipsisTooltip>
          );
        },
      },
    ];
  }, [device]);

  // TODO: Refactor these device dimensions tables to use shared styles.
  const table = useFCDataTable({
    columns,
    data: rows,
    getRowId: (row) => row.id,
    enableTopToolbar: true,
    enableDensityToggle: true,
    enableFullScreenToggle: false,
    enableHiding: false,
    enableColumnActions: false,
    enableColumnFilters: false,
    enablePagination: false,
    enableSorting: false,
    enableStickyHeader: true,
    enableBottomToolbar: false,
    enableRowSelection: false,
    muiTableHeadRowProps: {
      sx: { minHeight: 'unset' },
    },
    muiTableContainerProps: {
      sx: {
        maxHeight: '600px', // or whatever constraints are appropriate
        '--cell-padding-horizontal': '16px',
        '& .Mui-TableHeadCell-Content': {
          minHeight: 'unset !important',
        },
      },
    },
  });

  if (device?.id === undefined) {
    return <></>;
  }

  return (
    device && (
      <>
        <BotInformation swarmingHost={swarmingHost || ''} botId={botId || ''} />

        <BotState swarmingHost={swarmingHost || ''} botId={botId || ''} />

        <Typography variant="h5" sx={{ mt: 4 }}>
          Device Dimensions
        </Typography>
        <div style={{ width: '100%', height: '100%' }}>
          <MaterialReactTable table={table} />
        </div>
      </>
    )
  );
};
