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

import { Alert, Link } from '@mui/material';
import _ from 'lodash';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_TableInstance,
} from 'material-react-table';
import { useEffect, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { RequestRepair } from '@/fleet/components/actions/request_repair/request_repair';
import { BrowserDeviceToRepair } from '@/fleet/components/actions/request_repair/request_repair_browser_config';
import {
  extractHostname,
  extractPool,
} from '@/fleet/components/actions/request_repair/request_repair_utils';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { FleetTableMeta } from '@/fleet/components/fc_data_table/types';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useFleetMRTState } from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { getFeatureFlag } from '@/fleet/config/features';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  BrowserDevice,
  ListBrowserDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AdminTasksAlert } from '../common/admin_tasks_alert';

import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';
import { BrowserSummaryHeader } from './browser_summary_header';
import { useBrowserDeviceDimensions } from './use_browser_device_dimensions';
import { useBrowserFilters } from './use_browser_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

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

export const BrowserDevicesPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const dimensionsQuery = useBrowserDeviceDimensions();

  const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');

  const columnIds = useMemo(() => {
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols = urlCols.length > 0 ? urlCols : BROWSER_DEFAULT_COLUMNS;
    return getBrowserColumnIds(dimensionsQuery.data, requiredCols);
  }, [dimensionsQuery.data, columnsParamStr]);

  useEffect(() => {
    const filters = searchParams.get('filters') || '';
    const needsRewrite =
      filters.includes('swarming_labels.') || filters.includes('ufs_labels.');

    const columns = searchParams.getAll(COLUMNS_PARAM_KEY);
    const columnsNeedRewrite = columns.some(
      (col) =>
        col.startsWith('swarming_labels.') || col.startsWith('ufs_labels.'),
    );

    if (needsRewrite || columnsNeedRewrite) {
      const newSearchParams = new URLSearchParams(searchParams);

      if (needsRewrite) {
        newSearchParams.set(
          'filters',
          filters
            .replace(/swarming_labels\./g, 'sw.')
            .replace(/ufs_labels\./g, 'ufs.'),
        );
      }

      if (columnsNeedRewrite) {
        const newColumns = columns.map((col) =>
          col.replace('swarming_labels.', 'sw.').replace('ufs_labels.', 'ufs.'),
        );
        newSearchParams.delete(COLUMNS_PARAM_KEY);
        for (const col of newColumns) {
          newSearchParams.append(COLUMNS_PARAM_KEY, col);
        }
      }

      setSearchParams(newSearchParams);
    }
  }, [searchParams, setSearchParams]);

  const [warnings, addWarning] = useWarnings();

  useEffect(() => {
    if (dimensionsQuery.isPending) return;

    const missingParamsColumns = getWrongColumnsFromParams(
      searchParams,
      columnIds,
      BROWSER_DEFAULT_COLUMNS,
    );
    if (missingParamsColumns.length === 0) return;
    addWarning(
      'The following columns are not available: ' +
        missingParamsColumns?.join(', '),
    );
    for (const col of missingParamsColumns) {
      searchParams.delete(COLUMNS_PARAM_KEY, col);
    }
    if (searchParams.getAll(COLUMNS_PARAM_KEY).length <= 1)
      searchParams.delete(COLUMNS_PARAM_KEY);

    setSearchParams(searchParams);
  }, [
    addWarning,
    columnIds,
    dimensionsQuery.isPending,
    searchParams,
    setSearchParams,
  ]);

  const columnsRecord = useMemo(
    () => Object.fromEntries(columnIds.map((id) => [id, getBrowserColumn(id)])),
    [columnIds],
  );

  const visibleColumns = useMemo(
    () => Object.values(columnsRecord),
    [columnsRecord],
  );

  const {
    filterValues,
    aip160,
    warnings: filterWarnings,
    isLoading,
  } = useBrowserFilters(() => {
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  });

  const request = ListBrowserDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: filterWarnings.length > 0 ? '' : aip160(),
  });

  const devicesQuery = useBrowserDevices(request);

  const {
    devices = [],
    nextPageToken = '',
    totalSize = 0,
  } = devicesQuery.data || {};

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    filterValues: filterValues,
    visibleColumns: visibleColumns,
    orderByParam,
    localStorageKey: BROWSER_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: BROWSER_DEFAULT_COLUMNS,
    platform: Platform.CHROMIUM,
  });

  const meta = useMemo<FleetTableMeta<BrowserDevice>>(
    () => ({
      availableColumns: fleetMrtState.allColumns,
      visibleColumnIds: fleetMrtState.visibleColumnIds,
      onToggleColumn: fleetMrtState.mrtColumnManager.onToggleColumn,
      selectOnlyColumn: fleetMrtState.mrtColumnManager.selectOnlyColumn,
      resetDefaultColumns: fleetMrtState.mrtColumnManager.resetDefaultColumns,
      resetColumnWidths: fleetMrtState.resetColumnWidths,
      devices,
      nextPageToken,
      totalSize,
      pagerCtx,
      searchParams,
      goToPrevPage: fleetMrtState.goToPrevPage,
      goToNextPage: fleetMrtState.goToNextPage,
      onRowsPerPageChange: fleetMrtState.onRowsPerPageChange,
    }),
    [
      fleetMrtState.allColumns,
      fleetMrtState.visibleColumnIds,
      fleetMrtState.mrtColumnManager,
      fleetMrtState.resetColumnWidths,
      devices,
      nextPageToken,
      totalSize,
      pagerCtx,
      searchParams,
      fleetMrtState.goToPrevPage,
      fleetMrtState.goToNextPage,
      fleetMrtState.onRowsPerPageChange,
    ],
  );

  const table = useFCDataTable({
    columns: fleetMrtState.enrichedColumns as MRT_ColumnDef<BrowserDevice>[],
    data: devices as BrowserDevice[],
    meta,
    filterValues: filterValues,
    displayColumnDefOptions: {
      'mrt-row-select': {
        size: 40,
        minSize: 40,
        maxSize: 40,
        grow: false,
      },
    },
    enableColumnResizing: true,
    enablePagination: false,
    enableRowSelection: true,
    positionToolbarAlertBanner: 'none',
    manualFiltering: true,

    manualSorting: true,
    manualPagination: true,
    getRowId: (row) => row.id,
    rowCount: totalSize,
    state: {
      isLoading: devicesQuery.isPending || devicesQuery.isPlaceholderData,
      columnVisibility: fleetMrtState.columnVisibility,
      sorting: fleetMrtState.sorting,
      rowSelection: fleetMrtState.rowSelection,
      columnSizing: fleetMrtState.columnSizing,
    },
    onColumnVisibilityChange:
      fleetMrtState.mrtColumnManager.setColumnVisibility,
    onSortingChange: fleetMrtState.onSortingChange,
    onRowSelectionChange: fleetMrtState.onRowSelectionChange,
    onColumnSizingChange: fleetMrtState.onColumnSizingChange,

    muiTopToolbarProps: {
      sx: {
        '& [aria-label="Show/Hide filters"]': {
          display: 'none',
        },
      },
    },
    renderTopToolbarCustomActions: ({ table }) => (
      <FleetTopToolbar table={table}>
        <BrowserActions table={table} />
      </FleetTopToolbar>
    ),
    renderBottomToolbarCustomActions: ({ table }) => (
      <FleetBottomToolbar table={table} />
    ),
  });

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={[...filterWarnings, ...warnings]} />
      <BrowserSummaryHeader />
      <AdminTasksAlert />
      <Alert
        severity="info"
        sx={{
          marginTop: '24px',
        }}
      >
        Currently, this page only displays physical devices. virtualized
        hardware (VMs and GCE instances) will be onboarded by the end of Q2 or
        early Q3 (tracking bug:{' '}
        <Link href="http://b/503171517" target="_blank" rel="noreferrer">
          b/503171517
        </Link>
        ).
      </Alert>
      <div
        css={{
          marginTop: 24,
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 28,
          borderRadius: 4,
        }}
      >
        <FilterBar
          filterCategoryDatas={Object.values(filterValues || {})}
          isLoading={isLoading}
          searchPlaceholder='Add a filter (e.g. "os:Linux" or "sw.pool:default")'
        />
      </div>
      <div
        css={{
          borderRadius: 4,
          marginTop: 24,
        }}
      >
        <MaterialReactTable table={table} />
      </div>
    </div>
  );
};

export function Component() {
  const isSupported = getFeatureFlag('BrowserListDevices');

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary key="fleet-device-list-page">
        <LoggedInBoundary>
          {isSupported ? (
            <BrowserDevicesPage />
          ) : (
            <PlatformNotAvailable
              availablePlatforms={[Platform.CHROMEOS, Platform.ANDROID]}
            />
          )}
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
