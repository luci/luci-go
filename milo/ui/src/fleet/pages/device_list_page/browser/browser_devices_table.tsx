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

import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_RowSelectionState,
  MRT_TableInstance,
  MRT_TableOptions,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { RequestRepair } from '@/fleet/components/actions/request_repair/request_repair';
import { BrowserDeviceToRepair } from '@/fleet/components/actions/request_repair/request_repair_browser_config';
import {
  extractHostname,
  extractPool,
} from '@/fleet/components/actions/request_repair/request_repair_utils';
import { MrtColumnManager } from '@/fleet/components/columns/use_mrt_column_management';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  BrowserDevice,
  ListBrowserDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { BrowserColumnDef } from './browser_fields';
import { useBrowserFilters } from './use_browser_filters';

const BrowserActions = ({
  table,
}: {
  table: MRT_TableInstance<BrowserDevice>;
}) => {
  const selectedRows = table.getSelectedRowModel().rows.map((r) => r.original);

  const selectedDevices: BrowserDeviceToRepair[] = selectedRows.map((row) => ({
    id: row.id,
    hostname: extractHostname(row),
    pool: extractPool(row),
    zone: row.ufsLabels?.['zone']?.values?.[0],
    serialNumber:
      row.ufsLabels?.['serial_number']?.values?.[0] ||
      row.ufsLabels?.['serialNumber']?.values?.[0],
    type: row.swarmingLabels?.['type']?.values?.[0],
    model: row.swarmingLabels?.['model']?.values?.[0],
    status:
      row.swarmingLabels?.['status']?.values?.[0] ||
      row.swarmingLabels?.['dut_state']?.values?.[0],
    lastSeen: row.swarmingLabels?.['last_seen']?.values?.[0],
  }));

  return selectedRows.length > 0 ? (
    <RequestRepair
      selectedItems={selectedDevices}
      platform={Platform.CHROMIUM}
    />
  ) : null;
};

interface BrowserTableProps {
  mrtColumnManager: MrtColumnManager<BrowserColumnDef>;
}

export const BrowserDevicesTable = ({
  mrtColumnManager,
}: BrowserTableProps) => {
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

  const {
    filterValues,
    aip160,
    warnings: filterWarnings,
  } = useBrowserFilters(() => {
    // Dummy onApply, handled by the page component.
  });

  const aip160Filter = filterWarnings.length > 0 ? '' : aip160();

  const request = useMemo(
    () =>
      ListBrowserDevicesRequest.fromPartial({
        pageSize,
        pageToken,
        orderBy: orderByParam,
        filter: aip160Filter,
      }),
    [pageSize, pageToken, orderByParam, aip160Filter],
  );

  const devicesQuery = useBrowserDevices(request);

  const {
    devices = [],
    nextPageToken = '',
    totalSize = 0,
  } = devicesQuery.data || {};

  const [rowSelection, setRowSelection] = useState<MRT_RowSelectionState>({});

  const { columnSizing, onColumnSizingChange, resetColumnWidths } =
    useMrtColumnSizing(BROWSER_DEVICES_LOCAL_STORAGE_KEY);

  // Clear row selection when filters change to prevent performing bulk actions
  // on hidden/invisible rows.
  useEffect(() => {
    setRowSelection({});
  }, [filterValues]);

  const tableOptions: MRT_TableOptions<BrowserDevice> = useMemo(
    () => ({
      columns: mrtColumnManager.columns as MRT_ColumnDef<BrowserDevice>[],
      data: devices as BrowserDevice[],
      displayColumnDefOptions: {
        'mrt-row-select': {
          size: 40,
          minSize: 40,
          maxSize: 40,
          grow: false,
        },
      },
      enableColumnResizing: true,
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
          availableColumns={mrtColumnManager.allColumns}
          visibleColumnIds={mrtColumnManager.visibleColumnIds}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          resetColumnWidths={resetColumnWidths}
        >
          <BrowserActions table={table} />
        </FleetTopToolbar>
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={totalSize}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      getRowId: (row: BrowserDevice) => row.id,
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
          ...mrtColumnManager.columns.map((col) => col.id as string),
        ],
      },
      manualFiltering: true,
      manualPagination: true,
      onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
      onColumnSizingChange,
    }),
    [
      mrtColumnManager,
      resetColumnWidths,
      devices,
      totalSize,
      nextPageToken,
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
    ],
  );

  const table = useFCDataTable({
    ...tableOptions,
    error: devicesQuery.error
      ? getErrorMessage(devicesQuery.error, 'fetch devices')
      : undefined,
  });

  return (
    <>
      <MaterialReactTable table={table} />
    </>
  );
};
