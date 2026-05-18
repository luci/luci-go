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
  MRT_ColumnDef,
  MRT_TableInstance,
  MRT_TableOptions,
  MRT_VisibilityState,
} from 'material-react-table';
import { useEffect, useMemo, useRef } from 'react';

import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { RunAutorepair } from '@/fleet/components/actions/autorepair/run_autorepair';
import { RunDeploy } from '@/fleet/components/actions/deploy/run_deploy';
import { RequestRepair } from '@/fleet/components/actions/request_repair/request_repair';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { FleetTableMeta } from '@/fleet/components/fc_data_table/types';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useFleetMRTState } from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { CHROMEOS_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { CHROMEOS_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useDevices } from '@/fleet/hooks/use_devices';
import { extractDutId, extractDutState } from '@/fleet/utils/devices';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountDevicesRequest,
  ListDevicesRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { ChromeOSExportButton } from './chromeos_export_button';
import { getLabelValue, getLabelValues } from './chromeos_fields_config';
import { ChromeOSDevice, ChromeOSColumnDef } from './chromeos_types';
import { useChromeOSCurrentTasks } from './use_chromeos_current_tasks';
import { useChromeOSFilters } from './use_chromeos_filters';

const ChromeOSActions = ({
  table,
}: {
  table: MRT_TableInstance<ChromeOSDevice>;
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
    return <ChromeOSExportButton table={table} selectedRowIds={[]} />;
  }

  return (
    <>
      <RunAutorepair selectedDuts={selectedDuts} />
      <RunDeploy selectedDuts={selectedDuts} />
      <RequestRepair
        selectedItems={selectedDuts}
        platform={Platform.CHROMEOS}
      />
      <ChromeOSExportButton
        table={table}
        selectedRowIds={Object.keys(table.getState().rowSelection)}
      />
    </>
  );
};

interface ChromeOSTableProps {
  availableColumns: ChromeOSColumnDef[];
  visibleColumns: ChromeOSColumnDef[];
}

export const ChromeOSTable = ({
  availableColumns,
  visibleColumns,
}: ChromeOSTableProps) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const { trackEvent } = useGoogleAnalytics();
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100, 500, 1000],
    defaultPageSize: 100,
  });

  const client = useFleetConsoleClient();
  const filterCategoryDatas = useChromeOSFilters(() => {
    // Dummy onApply for now, URL updates are handled by the page component or useFilters.
  });

  const columnFiltersRef = useRef<{ id: string; value: unknown }[]>([]);

  const countQuery = useQuery({
    ...client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: filterCategoryDatas.aip160(),
        platform: Platform.CHROMEOS,
      }),
    ),
  });

  const request = useMemo(
    () =>
      ListDevicesRequest.fromPartial({
        pageSize: getPageSize(pagerCtx, searchParams),
        pageToken: getPageToken(pagerCtx, searchParams),
        orderBy: orderByParam,
        filter: filterCategoryDatas.aip160(),
        platform: Platform.CHROMEOS,
      }),
    [pagerCtx, searchParams, orderByParam, filterCategoryDatas],
  );

  const devicesQuery = useDevices(request);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};

  const currentTasks = useChromeOSCurrentTasks(devices);

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    filterValues: filterCategoryDatas.filterValues,
    visibleColumns: visibleColumns,
    orderByParam,
    localStorageKey: CHROMEOS_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: CHROMEOS_DEFAULT_COLUMNS,
    platform: Platform.CHROMEOS,
    isLoadingOptions: filterCategoryDatas.isLoading,
    onColumnFiltersChangeOverride: () => {
      trackEvent('filter_changed', {
        componentName: 'device_list_filter',
      });
    },
  });

  const {
    enrichedColumns,
    onRowSelectionChange,
    onSortingChange,
    onColumnFiltersChange,
    rowSelection,
    sorting,
    columnFilters,
    columnVisibility,
    mrtColumnManager: { setColumnVisibility },
  } = fleetMrtState;

  const rows = useMemo(() => {
    const baseRows = currentTasks.isPending
      ? devices.map((d) => ({
          ...d,
          current_task: undefined,
        }))
      : devices.map((d) => ({
          ...d,
          current_task: currentTasks.map.get(extractDutId(d)),
        }));

    return baseRows;
  }, [devices, currentTasks.map, currentTasks.isPending]);

  useEffect(() => {
    columnFiltersRef.current = columnFilters;
  }, [columnFilters]);

  const meta = useMemo<FleetTableMeta<ChromeOSDevice>>(
    () => ({
      availableColumns: availableColumns.map((col) => ({
        id: col.accessorKey as string,
        label: col.header,
      })),
      visibleColumnIds: enrichedColumns
        .filter(
          (col) => columnVisibility[col.id || (col.accessorKey as string)],
        )
        .map((col) => (col.id || col.accessorKey) as string),
      onToggleColumn: (id) => {
        setColumnVisibility((prev: MRT_VisibilityState) => ({
          ...prev,
          [id]: !prev[id],
        }));
      },
      resetDefaultColumns: () => {
        const nextVisibility = { ...columnVisibility };
        CHROMEOS_DEFAULT_COLUMNS.forEach((id) => {
          nextVisibility[id] = true;
        });
        setColumnVisibility(nextVisibility);
      },
      devicesQuery,
      nextPageToken,
      devices,
      pagerCtx,
      searchParams,
      goToPrevPage: fleetMrtState.goToPrevPage,
      goToNextPage: fleetMrtState.goToNextPage,
      onRowsPerPageChange: fleetMrtState.onRowsPerPageChange,
      countQuery,
      totalSize: countQuery?.data?.total,
    }),
    [
      availableColumns,
      enrichedColumns,
      columnVisibility,
      setColumnVisibility,
      devicesQuery,
      nextPageToken,
      devices,
      pagerCtx,
      searchParams,
      countQuery,
      fleetMrtState.goToPrevPage,
      fleetMrtState.goToNextPage,
      fleetMrtState.onRowsPerPageChange,
    ],
  );

  const tableOptions: MRT_TableOptions<ChromeOSDevice> = useMemo(
    () => ({
      columns: enrichedColumns as MRT_ColumnDef<ChromeOSDevice>[],
      data: rows,
      displayColumnDefOptions: {
        'mrt-row-select': {
          size: 40,
        },
      },
      enableRowSelection: true,
      positionToolbarAlertBanner: 'none',
      renderTopToolbarCustomActions: ({ table }) => (
        <FleetTopToolbar table={table}>
          <ChromeOSActions table={table} />
        </FleetTopToolbar>
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar table={table} />
      ),
      getRowId: (row: ChromeOSDevice) => row.id,
      onRowSelectionChange,
      onSortingChange,
      onColumnFiltersChange,
      enablePagination: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      filterValues: filterCategoryDatas.filterValues,
      meta,
      state: {
        rowSelection,
        sorting,
        columnFilters,
        columnVisibility,
        isLoading: devicesQuery.isPending && !devicesQuery.isPlaceholderData,
        showProgressBars: devicesQuery.isFetching,
        columnOrder: [
          'mrt-row-select',
          ...enrichedColumns
            .filter((col) => {
              const id = (col.id || col.accessorKey) as string;
              return columnVisibility[id];
            })
            .map((col) => (col.id || col.accessorKey) as string),
        ],
      },
      manualFiltering: true,
      manualPagination: true,
      onColumnVisibilityChange: setColumnVisibility,
    }),
    [
      enrichedColumns,
      rows,
      onRowSelectionChange,
      onSortingChange,
      onColumnFiltersChange,
      filterCategoryDatas.filterValues,
      meta,
      rowSelection,
      sorting,
      columnFilters,
      columnVisibility,
      devicesQuery,
      setColumnVisibility,
    ],
  );

  const table = useFCDataTable(tableOptions);

  return (
    <>
      <MaterialReactTable table={table} />
    </>
  );
};
