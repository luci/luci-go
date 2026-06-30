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

import { useQuery } from '@tanstack/react-query';
import {
  MaterialReactTable,
  MRT_RowSelectionState,
  MRT_TableInstance,
  MRT_TableOptions,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { RunAutorepair } from '@/fleet/components/actions/autorepair/run_autorepair';
import { RunDeploy } from '@/fleet/components/actions/deploy/run_deploy';
import { RequestRepair } from '@/fleet/components/actions/request_repair/request_repair';
import { RunReserve } from '@/fleet/components/actions/reserve/run_reserve';
import { MrtColumnManager } from '@/fleet/components/columns/use_mrt_column_management';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { CHROMEOS_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useDevices } from '@/fleet/hooks/use_devices';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { extractDutId, extractDutState } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  CountDevicesRequest,
  ListDevicesRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { ChromeOSExportButton } from './chromeos_export_button';
import { getLabelValue, getLabelValues } from './chromeos_fields_config';
import { ChromeOSColumnDef, ChromeOSDevice } from './chromeos_types';
import { useChromeOSCurrentTasks } from './use_chromeos_current_tasks';
import { useChromeOSFilters } from './use_chromeos_filters';

const ChromeOSActions = ({
  table,
  filter,
}: {
  table: MRT_TableInstance<ChromeOSDevice>;
  filter: string;
}) => {
  const selectedModelRows = table.getSelectedRowModel().rows;
  const selectedRows = selectedModelRows.map((r) => r.original);

  const selectedDuts = selectedRows.map((row) => ({
    name: `${row.id}`,
    dutId: `${row.dutId}`,
    state: extractDutState(row),
    pool: getLabelValue(row, 'label-pool'),
    board: getLabelValue(row, 'label-board'),
    model: getLabelValue(row, 'label-model'),
    namespace: getLabelValues(row, 'ufs_namespace'),
  }));

  if (selectedRows.length === 0) {
    return (
      <ChromeOSExportButton table={table} selectedRowIds={[]} filter={filter} />
    );
  }

  return (
    <>
      <RunAutorepair selectedDuts={selectedDuts} />
      <RunDeploy selectedDuts={selectedDuts} />
      <RequestRepair
        selectedItems={selectedDuts}
        platform={Platform.CHROMEOS}
      />
      <RunReserve selectedDuts={selectedDuts} />
      <ChromeOSExportButton
        table={table}
        selectedRowIds={Object.keys(table.getState().rowSelection)}
        filter={filter}
      />
    </>
  );
};

interface ChromeOSTableProps {
  mrtColumnManager: MrtColumnManager<ChromeOSColumnDef>;
}

export const ChromeOSTable = ({ mrtColumnManager }: ChromeOSTableProps) => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100, 500, 1000],
    defaultPageSize: 100,
  });

  const { pageSize, pageToken } = usePager(pagerCtx);

  const [sorting, onSortingChange, orderByParam] = useMrtSortingState(
    mrtColumnManager.columns,
    pagerCtx,
  );

  const client = useFleetConsoleClient();
  const filterCategoryDatas = useChromeOSFilters(() => {
    // Dummy onApply for now, URL updates are handled by the page component or useFilters.
  });

  const countQuery = useQuery({
    ...client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: filterCategoryDatas.aip160(),
        platform: Platform.CHROMEOS,
      }),
    ),
  });

  const aip160Filter = filterCategoryDatas.aip160();

  const request = useMemo(
    () =>
      ListDevicesRequest.fromPartial({
        pageSize,
        pageToken,
        orderBy: orderByParam,
        filter: aip160Filter,
        platform: Platform.CHROMEOS,
      }),
    [pageSize, pageToken, orderByParam, aip160Filter],
  );

  const devicesQuery = useDevices(request);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};

  const currentTasks = useChromeOSCurrentTasks(devices);

  const [rowSelection, setRowSelection] = useState<MRT_RowSelectionState>({});

  const { columnSizing, onColumnSizingChange, resetColumnWidths } =
    useMrtColumnSizing(CHROMEOS_DEVICES_LOCAL_STORAGE_KEY);

  // Clear row selection when filters change to prevent performing bulk actions
  // on hidden/invisible rows.
  useEffect(() => {
    setRowSelection({});
  }, [filterCategoryDatas.filterValues]);

  const rows = useMemo(() => {
    const baseRows = currentTasks.isPending
      ? devices.map((d) => ({
          ...d,
          current_task: undefined,
        }))
      : devices.map((d) => ({
          ...d,
          current_task: currentTasks.tasks[extractDutId(d)],
        }));

    return baseRows;
  }, [devices, currentTasks.tasks, currentTasks.isPending]);

  const tableOptions: MRT_TableOptions<ChromeOSDevice> = useMemo(
    () => ({
      columns: mrtColumnManager.columns,
      data: rows as ChromeOSDevice[],
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
          availableColumns={mrtColumnManager.allColumns}
          visibleColumnIds={mrtColumnManager.columns
            .filter((col) => mrtColumnManager.columnVisibility[col.id])
            .map((col) => col.id)}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          resetColumnWidths={resetColumnWidths}
        >
          <ChromeOSActions table={table} filter={aip160Filter} />
        </FleetTopToolbar>
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={countQuery?.data?.total}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      getRowId: (row: ChromeOSDevice) => row.id,
      onRowSelectionChange: setRowSelection,
      onSortingChange,
      enablePagination: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      filterValues: filterCategoryDatas.filterValues,
      state: {
        rowSelection,
        sorting,
        columnVisibility: mrtColumnManager.columnVisibility,
        columnSizing,
        isLoading: devicesQuery.isPending && !devicesQuery.isPlaceholderData,
        showProgressBars: devicesQuery.isFetching,
        columnOrder: [
          'mrt-row-select',
          ...mrtColumnManager.columns.map((col) => col.id),
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
      rows,
      onSortingChange,
      filterCategoryDatas.filterValues,
      rowSelection,
      sorting,
      devicesQuery.isPending,
      devicesQuery.isPlaceholderData,
      devicesQuery.isFetching,
      countQuery?.data?.total,
      nextPageToken,
      pagerCtx,
      columnSizing,
      onColumnSizingChange,
      aip160Filter,
    ],
  );

  const table = useFCDataTable({
    ...tableOptions,
    error:
      devicesQuery.error || countQuery.error
        ? getErrorMessage(
            devicesQuery.error || countQuery.error,
            'fetch devices',
          )
        : undefined,
  });

  return (
    <>
      <MaterialReactTable table={table} />
    </>
  );
};
