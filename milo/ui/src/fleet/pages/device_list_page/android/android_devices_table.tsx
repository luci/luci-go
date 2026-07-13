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

import { Alert, AlertTitle } from '@mui/material';
import {
  MaterialReactTable,
  MRT_RowSelectionState,
  MRT_TableOptions,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import {
  getColumnId,
  MrtColumnManager,
} from '@/fleet/components/columns/use_mrt_column_management';
import { FleetCSVExportButton } from '@/fleet/components/export/fleet_csv_export_button';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  AndroidDevice,
  ListDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { ExportAndroidDevicesToCSVRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AndroidColumnDef } from './android_fields';
import { useAndroidFilters } from './use_android_filters';

interface AndroidTableProps {
  mrtColumnManager: MrtColumnManager<AndroidColumnDef>;
  availableColumns: { id: string; label: string }[];
}

export const AndroidDevicesTable = ({
  mrtColumnManager,
  availableColumns,
}: AndroidTableProps) => {
  const client = useFleetConsoleClient();
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100, 500, 1000],
    defaultPageSize: 100,
  });

  const { pageSize, pageToken } = usePager(pagerCtx);

  const [sorting, onSortingChange, orderByParam] = useMrtSortingState(
    mrtColumnManager.columns.map((c) => ({
      id: c.id || (c.accessorKey as string) || '',
      orderByField: c.orderByField,
    })),
    pagerCtx,
  );

  const [rowSelection, setRowSelection] = useState<MRT_RowSelectionState>({});

  const { filterValues, aip160 } = useAndroidFilters(() => {});

  const aip160Filter = aip160();

  // Clear row selection when filters change to prevent performing bulk actions
  // on hidden/invisible rows.
  useEffect(() => {
    setRowSelection({});
  }, [filterValues]);

  const request = useMemo(
    () =>
      ListDevicesRequest.fromPartial({
        pageSize,
        pageToken,
        orderBy: orderByParam,
        filter: aip160Filter,
        platform: Platform.ANDROID,
      }),
    [pageSize, pageToken, orderByParam, aip160Filter],
  );

  const devicesQuery = useAndroidDevices(request);

  const { devices = [] } = devicesQuery.data || {};

  const { columnSizing, onColumnSizingChange, resetColumnWidths } =
    useMrtColumnSizing(ANDROID_DEVICES_LOCAL_STORAGE_KEY);

  const visibleColumns = useMemo(() => {
    return mrtColumnManager.columns.filter((col) => {
      const id = getColumnId(col);
      return id && mrtColumnManager.columnVisibility[id];
    });
  }, [mrtColumnManager.columns, mrtColumnManager.columnVisibility]);

  const tableOptions: MRT_TableOptions<AndroidDevice> = useMemo(
    () => ({
      columns: visibleColumns,
      data: devices as AndroidDevice[],
      displayColumnDefOptions: {
        'mrt-row-select': {
          size: 40,
        },
      },
      enableRowSelection: true,
      muiSelectCheckboxProps: ({ row }) => ({
        inputProps: {
          'data-testid': `select-checkbox-${row.id}`,
        } as React.InputHTMLAttributes<HTMLInputElement> & {
          'data-testid'?: string;
        },
      }),
      positionToolbarAlertBanner: 'none',
      renderTopToolbarCustomActions: ({ table }) => (
        <FleetTopToolbar
          table={table}
          availableColumns={availableColumns}
          visibleColumnIds={mrtColumnManager.columns
            .filter((col) => {
              const id = getColumnId(col);
              return id && mrtColumnManager.columnVisibility[id];
            })
            .map((col) => getColumnId(col))}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          resetColumnWidths={resetColumnWidths}
        >
          <FleetCSVExportButton
            table={table}
            filter={aip160Filter}
            fileName="fleet_console_android_devices"
            onExport={(cols, filterStr, ids) =>
              client.ExportAndroidDevicesToCSV(
                ExportAndroidDevicesToCSVRequest.fromPartial({
                  columns: cols,
                  orderBy: orderByParam,
                  filter: filterStr,
                  ids,
                }),
              )
            }
          />
        </FleetTopToolbar>
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={devicesQuery.data?.totalSize}
          nextPageToken={devicesQuery.data?.nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      getRowId: (row: AndroidDevice) => row.id,
      onRowSelectionChange: setRowSelection,
      onSortingChange,
      enablePagination: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      filterValues,
      state: {
        rowSelection,
        sorting,
        columnVisibility: mrtColumnManager.columnVisibility,
        columnSizing,
        isLoading: devicesQuery.isPending && !devicesQuery.isPlaceholderData,
        showProgressBars: devicesQuery.isFetching,
        columnOrder: [
          'mrt-row-select',
          ...mrtColumnManager.columns
            .filter((col) => {
              const id = getColumnId(col);
              return id && mrtColumnManager.columnVisibility[id];
            })
            .map((col) => getColumnId(col)),
        ],
      },
      manualFiltering: true,
      manualPagination: true,
      onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
      onColumnSizingChange,
    }),
    [
      visibleColumns,
      devices,
      availableColumns,
      mrtColumnManager,
      resetColumnWidths,
      devicesQuery.data?.totalSize,
      devicesQuery.data?.nextPageToken,
      pagerCtx,
      onSortingChange,
      filterValues,
      rowSelection,
      sorting,
      devicesQuery.isPending,
      devicesQuery.isPlaceholderData,
      devicesQuery.isFetching,
      columnSizing,
      onColumnSizingChange,
      aip160Filter,
      client,
      orderByParam,
    ],
  );

  const table = useFCDataTable({
    ...tableOptions,
    error: devicesQuery.error
      ? getErrorMessage(devicesQuery.error, 'fetch devices')
      : undefined,
  });

  if (devicesQuery.isError) {
    return (
      <Alert severity="error">
        <AlertTitle>Error Loading Android Devices</AlertTitle>
        {getErrorMessage(devicesQuery.error, 'fetch device list')}
      </Alert>
    );
  }

  return (
    <>
      <MaterialReactTable table={table} />
    </>
  );
};
