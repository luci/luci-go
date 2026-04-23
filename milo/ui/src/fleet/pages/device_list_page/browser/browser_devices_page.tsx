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

import { Chip } from '@mui/material';
import _ from 'lodash';
import { MaterialReactTable, MRT_ColumnDef } from 'material-react-table';
import { useCallback, useEffect, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { emptyPageTokenUpdater } from '@/common/components/params_pager';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { FleetTableMeta } from '@/fleet/components/fc_data_table/types';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  FleetColumnDefExt,
  useFleetMRTState,
} from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import {
  GetFiltersResult,
  stringifyFilters,
} from '@/fleet/components/filter_dropdown/parser/parser';
import {
  filtersUpdater,
  getFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils';
import { FILTERS_PARAM_KEY } from '@/fleet/components/filter_dropdown/search_param_utils';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  FilterCategory,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { getFeatureFlag } from '@/fleet/config/features';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import {
  computeSelectedOptions,
  syncFilterCategory,
} from '@/fleet/utils/filters';
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
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AdminTasksAlert } from '../common/admin_tasks_alert';

import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';
import { BrowserSummaryHeader } from './browser_summary_header';
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
        const selectedOptions = getFilters(searchParams);
        if (!selectedOptions.error) {
          newSearchParams.set(
            'filters',
            stringifyFilters(selectedOptions.filters),
          );
        }
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
    if (!isDimensionsQueryProperlyLoaded) return {};

    const filters: Record<string, StringListFilterCategoryBuilder> = {};

    const addDimensions = (
      dimensions: Record<string, { values: readonly string[] }>,
      getColumnId: (k: string) => string,
    ) => {
      for (const [filterKey, filterValues] of Object.entries(dimensions)) {
        const key = getColumnId(filterKey);
        const label = (columnsRecord[key]?.header || key) as string;
        const value = columnsRecord[key]?.filterByField || key;

        filters[value] = new StringListFilterCategoryBuilder()
          .setLabel(label)
          .setOptions([
            { label: BLANK_VALUE, value: BLANK_VALUE },
            ...(filterValues.values || [])
              .filter((v) => v !== '' && v !== BLANK_VALUE)
              .map((v) => ({ label: v, value: v })),
          ]);
      }
    };

    addDimensions(dimensionsQuery.data.baseDimensions, (k) => k);
    addDimensions(
      dimensionsQuery.data.swarmingLabels,
      (k) => `${BROWSER_SWARMING_SOURCE}."${k}"`,
    );
    addDimensions(
      dimensionsQuery.data.ufsLabels,
      (k) => `${BROWSER_UFS_SOURCE}."${k}"`,
    );

    return filters;
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data, columnsRecord]);

  const filterCategoryDatas = useFilters(loadedFilterOptions, {
    allowExtraKeys: !isDimensionsQueryProperlyLoaded,
  });

  const selectedOptions = useMemo<GetFiltersResult>(() => {
    if (filterCategoryDatas.parseError) {
      return {
        filters: undefined,
        error: new Error(filterCategoryDatas.parseError),
      };
    }
    return {
      filters: computeSelectedOptions(filterCategoryDatas.filterValues),
      error: undefined,
    };
  }, [filterCategoryDatas.filterValues, filterCategoryDatas.parseError]);

  const onColumnFiltersChangeOverride = useCallback(
    (newFilters: Record<string, string[]>) => {
      trackEvent('filter_changed', {
        componentName: 'device_list_filter',
      });
      if (!filterCategoryDatas.filterValues) return;

      const prevTableFilterKeys = Object.keys(
        selectedOptions.filters || {},
      ).map((id) => normalizeFilterKey(id));

      for (const [key, category] of Object.entries(
        filterCategoryDatas.filterValues as Record<string, FilterCategory>,
      )) {
        syncFilterCategory(key, category, newFilters, prevTableFilterKeys);
      }

      const currentAIP160 = filterCategoryDatas.getAip160String();
      setSearchParams((prev) => {
        let newParams = new URLSearchParams(prev);
        newParams.set(FILTERS_PARAM_KEY, currentAIP160);
        newParams = emptyPageTokenUpdater(pagerCtx)(newParams);
        return newParams;
      });
    },
    [
      filterCategoryDatas,
      pagerCtx,
      setSearchParams,
      trackEvent,
      selectedOptions.filters,
    ],
  );

  const request = ListBrowserDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: filterCategoryDatas.parseError ? '' : filterCategoryDatas.aip160,
  });

  const devicesQuery = useBrowserDevices(request);

  const {
    devices = [],
    nextPageToken = '',
    totalSize = 0,
  } = devicesQuery.data || {};

  const fallbackRealms = useMemo(() => {
    const realms = new Set<string>();
    devices.forEach((dev: BrowserDevice) => {
      if (dev.realm) realms.add(dev.realm);
    });
    return Array.from(realms).map((r) => ({ label: r, value: r }));
  }, [devices]);

  const filterOptionsConfig = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return [];

    const options = Object.entries(loadedFilterOptions).map(
      ([key, builder]) => {
        const b = builder as StringListFilterCategoryBuilder;
        return {
          label: b.label || key,
          type: 'string_list' as const,
          value: key,
          options: b.options || [],
        };
      },
    );

    if (!options.some((o) => o.value === 'realm')) {
      options.push({
        label: 'Realm',
        type: 'string_list' as const,
        value: 'realm',
        options: fallbackRealms,
      });
    }

    return options;
  }, [isDimensionsQueryProperlyLoaded, loadedFilterOptions, fallbackRealms]);

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    selectedOptions,
    filterOptionsConfig,
    onColumnFiltersChangeOverride,
    columnsList: columnsList as unknown as FleetColumnDefExt[],

    orderByParam,
    localStorageKey: BROWSER_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: BROWSER_DEFAULT_COLUMNS,
    platform: Platform.CHROMIUM,
  });

  const meta = useMemo<FleetTableMeta<BrowserDevice>>(
    () => ({
      allDimensionColumns: fleetMrtState.allColumns,
      visibleColumnIds: fleetMrtState.visibleColumnIds,
      onToggleColumn: fleetMrtState.mrtColumnManager.onToggleColumn,
      selectOnlyColumn: fleetMrtState.mrtColumnManager.selectOnlyColumn,
      resetDefaultColumns: fleetMrtState.mrtColumnManager.resetDefaultColumns,
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
      <FleetTopToolbar table={table} />
    ),
    renderBottomToolbarCustomActions: ({ table }) => (
      <FleetBottomToolbar table={table} />
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
          <FilterBar
            filterCategoryDatas={Object.values(
              filterCategoryDatas.filterValues || {},
            )}
            onApply={() => {
              trackEvent('filter_changed', {
                componentName: 'device_list_filter',
              });
            }}
            isLoading={
              dimensionsQuery.isPending ||
              filterCategoryDatas.filterValues === undefined
            }
            searchPlaceholder='Add a filter (e.g. "os:Linux" or "sw.pool:default")'
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
