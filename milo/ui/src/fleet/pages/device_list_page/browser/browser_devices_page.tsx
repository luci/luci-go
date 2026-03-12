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

import { ViewColumnOutlined } from '@mui/icons-material';
import { Button, Chip, colors, TablePagination } from '@mui/material';
import _ from 'lodash';
import { MaterialReactTable } from 'material-react-table';
import { useEffect, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import { DeviceListFilterBar } from '@/fleet/components/device_table/device_list_filter_bar';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useFleetMRTState } from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import {
  filtersUpdater,
  getFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { getFeatureFlag } from '@/fleet/config/features';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { SelectedOptions } from '@/fleet/types';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  BrowserDevice,
  ListBrowserDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AdminTasksAlert } from '../common/admin_tasks_alert';
import { filterOptionsPlaceholder } from '../common/helpers';

import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';
import { BrowserSummaryHeader } from './browser_summary_header';
import { dimensionsToFilterOptions } from './dimensions_to_filter_options';
import { useBrowserDeviceDimensions } from './use_browser_device_dimensions';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

export const BrowserDevicesPage = () => {
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const selectedOptions = useMemo(
    () => getFilters(searchParams),
    [searchParams],
  );

  const onSelectedOptionsChange = (newSelectedOptions: SelectedOptions) => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });

    setSearchParams(filtersUpdater(newSelectedOptions));
  };

  const stringifiedSelectedOptions = selectedOptions.error
    ? ''
    : stringifyFilters(selectedOptions.filters);

  const dimensionsQuery = useBrowserDeviceDimensions();

  const request = ListBrowserDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: stringifiedSelectedOptions,
  });

  const devicesQuery = useBrowserDevices(request);

  const {
    devices = [],
    nextPageToken = '',
    totalSize = 0,
  } = devicesQuery.data || {};

  const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');

  const columnIds = useMemo(() => {
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols = urlCols.length > 0 ? urlCols : BROWSER_DEFAULT_COLUMNS;
    return getBrowserColumnIds(dimensionsQuery.data, requiredCols);
  }, [dimensionsQuery.data, columnsParamStr]);

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

  const columnsList = useMemo(
    () => Object.values(columnsRecord),
    [columnsRecord],
  );

  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.swarmingLabels &&
    dimensionsQuery.data.ufsLabels;

  const loadedFilterOptions = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return [];
    return dimensionsToFilterOptions(dimensionsQuery.data, columnsRecord);
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data, columnsRecord]);

  const placeholderFilterOptions = useMemo(() => {
    if (isDimensionsQueryProperlyLoaded) return [];
    return filterOptionsPlaceholder(
      selectedOptions.filters || {},
      columnsRecord,
    );
  }, [isDimensionsQueryProperlyLoaded, selectedOptions.filters, columnsRecord]);

  const filterOptionsConfig = isDimensionsQueryProperlyLoaded
    ? loadedFilterOptions
    : placeholderFilterOptions;

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    selectedOptions,
    filterOptionsConfig,
    columnsList,

    orderByParam,
    localStorageKey: BROWSER_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: BROWSER_DEFAULT_COLUMNS,
  });

  const table = useFCDataTable({
    columns: fleetMrtState.enrichedColumns,
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
      columnOrder: ['mrt-row-select', ...fleetMrtState.visibleColumnIds],
      sorting: fleetMrtState.sorting,
      columnFilters: fleetMrtState.columnFilters,
      rowSelection: fleetMrtState.rowSelection,
    },
    onColumnFiltersChange: fleetMrtState.onColumnFiltersChange,
    onColumnVisibilityChange:
      fleetMrtState.mrtColumnManager.setColumnVisibility,
    onSortingChange: fleetMrtState.onSortingChange,
    onRowSelectionChange: fleetMrtState.onRowSelectionChange,

    muiTopToolbarProps: {
      sx: {
        '& [aria-label="Show/Hide filters"]': {
          display: 'none',
        },
      },
    },
    renderTopToolbarCustomActions: ({ table }) => (
      <div
        css={{
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        <FCDataTableCopy table={table} />
        <ColumnsButton
          allColumns={fleetMrtState.allColumns}
          visibleColumns={fleetMrtState.visibleColumnIds}
          onToggleColumn={fleetMrtState.mrtColumnManager.onToggleColumn}
          resetDefaultColumns={
            fleetMrtState.mrtColumnManager.resetDefaultColumns
          }
          renderTrigger={({ onClick }, ref) => (
            <Button
              ref={ref}
              startIcon={<ViewColumnOutlined sx={{ fontSize: '20px' }} />}
              onClick={onClick}
              color="inherit"
              sx={{
                color: colors.grey[600],
                height: '40px',
                fontSize: '0.875rem',
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              Columns
            </Button>
          )}
        />
      </div>
    ),
    renderBottomToolbarCustomActions: () => (
      <div css={{ width: '100%', display: 'flex', justifyContent: 'flex-end' }}>
        <TablePagination
          component="div"
          count={totalSize === 0 && !nextPageToken && !devices.length ? 0 : -1}
          page={getCurrentPageIndex(pagerCtx)}
          rowsPerPage={getPageSize(pagerCtx, searchParams)}
          onPageChange={(_, page) => {
            const currentPage = getCurrentPageIndex(pagerCtx);
            const isPrevPage = page < currentPage;
            const isNextPage = page > currentPage;

            if (isPrevPage) {
              fleetMrtState.goToPrevPage();
            } else if (isNextPage) {
              fleetMrtState.goToNextPage(nextPageToken);
            }
          }}
          onRowsPerPageChange={(e) => {
            fleetMrtState.onRowsPerPageChange(Number(e.target.value));
          }}
          rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
          labelDisplayedRows={({ from, to }) => {
            if (totalSize !== undefined && totalSize > 0) {
              return `${from}-${to} of ${totalSize}`;
            }
            return `${from}-${to} of ${nextPageToken ? `more than ${to}` : to}`;
          }}
          slotProps={{
            actions: {
              previousButtonProps: {
                disabled: getCurrentPageIndex(pagerCtx) === 0,
                onClick: fleetMrtState.goToPrevPage,
              },
              nextButtonProps: {
                disabled: devices.length === 0 || nextPageToken === '',
                onClick: () => fleetMrtState.goToNextPage(nextPageToken),
              },
            } as NonNullable<
              React.ComponentProps<typeof TablePagination>['slotProps']
            >['actions'],
          }}
        />
      </div>
    ),
  });

  const validFilterByFields = useMemo(
    () =>
      new Set(
        Object.values(columnsRecord).map(
          (col) => col.filterByField || col.accessorKey || (col.id as string),
        ),
      ),
    [columnsRecord],
  );

  useEffect(() => {
    if (selectedOptions.error) return;
    if (!dimensionsQuery.isSuccess) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) =>
        isDimensionsQueryProperlyLoaded && !validFilterByFields.has(filterKey),
    );
    if (missingParamsFilters.length === 0) return;
    addWarning(
      'The following filters are not available: ' +
        missingParamsFilters?.join(', '),
    );
    for (const key of missingParamsFilters) {
      delete selectedOptions.filters[key];
    }
    setSearchParams(filtersUpdater(selectedOptions.filters));
  }, [
    addWarning,
    dimensionsQuery,
    selectedOptions,
    setSearchParams,
    validFilterByFields,
    isDimensionsQueryProperlyLoaded,
  ]);

  useEffect(() => {
    if (!selectedOptions.error) return;
    addWarning('Invalid filters');
    setSearchParams(filtersUpdater({}));
  }, [addWarning, selectedOptions.error, setSearchParams]);

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <BrowserSummaryHeader
        selectedOptions={selectedOptions.filters || {}}
        pagerContext={pagerCtx}
      />
      <AdminTasksAlert />
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
        {selectedOptions.error ? (
          <Chip
            variant="outlined"
            onDelete={() => setSearchParams(filtersUpdater({}))}
            label="Invalid filters"
            color="error"
          />
        ) : (
          <DeviceListFilterBar
            filterOptions={filterOptionsConfig}
            selectedOptions={selectedOptions.filters}
            onSelectedOptionsChange={onSelectedOptionsChange}
            isLoading={dimensionsQuery.isPending}
          />
        )}
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
