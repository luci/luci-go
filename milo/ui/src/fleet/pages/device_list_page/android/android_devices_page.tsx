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

import _ from 'lodash';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_TableOptions,
} from 'material-react-table';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { createFeatureFlag, useFeatureFlag } from '@/common/feature_flags';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { FleetTableMeta } from '@/fleet/components/fc_data_table/types';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  FleetColumnDefExt,
  useFleetMRTState,
} from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterState } from '@/fleet/components/filters/types';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getAndroidColumns } from '@/fleet/pages/device_list_page/android/android_columns';
import { ANDROID_COLUMN_OVERRIDES } from '@/fleet/pages/device_list_page/android/android_fields';
import { ANDROID_EXTRA_FILTERS } from '@/fleet/pages/device_list_page/android/android_filters';
import { AndroidSummaryHeader } from '@/fleet/pages/device_list_page/android/android_summary_header';
import { AdminTasksAlert } from '@/fleet/pages/device_list_page/common/admin_tasks_alert';
import { dimensionsToFilterOptions } from '@/fleet/pages/device_list_page/common/helpers';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  AndroidDevice,
  ListDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

const AVG_UTILIZATION_FEATURE = createFeatureFlag({
  name: 'avg-utilization-metrics',
  namespace: 'fleet-console-android',
  description:
    'Displays average device utilization metrics, columns, and filters.',
  percentage: 0,
  trackingBug: '',
});

const platform = Platform.ANDROID;

const computeSelectedOptions = (
  filterValues: Record<string, FilterCategory> | undefined,
): FilterState => {
  const filters: Record<string, string[]> = {};
  if (filterValues) {
    for (const [key, category] of Object.entries(filterValues)) {
      if (!category.isActive()) continue;
      let selected: string[] = [];
      if (category instanceof StringListFilterCategory) {
        selected = category.getSelectedOptions();
      }
      if (selected.length > 0) {
        const matchKey = normalizeFilterKey(key);
        const unquotedSelected = selected.map((k) =>
          typeof k === 'string' ? normalizeFilterKey(k) : k,
        );
        filters[matchKey] = unquotedSelected;
      }
    }
  }
  return { filters, error: undefined };
};

export const AndroidDevicesPage = () => {
  const showAvgUtilization = useFeatureFlag(AVG_UTILIZATION_FEATURE);
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const dimensionsQuery = useDeviceDimensions({ platform });

  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.labels;

  const filterOptions = useMemo(() => {
    const extraFilters: Record<
      string,
      FilterCategoryBuilder<FilterCategory>
    > = {
      ...ANDROID_EXTRA_FILTERS,
    };
    if (showAvgUtilization) {
      extraFilters['"average_7d"'] = new RangeFilterCategoryBuilder()
        .setLabel('7 Day Average Utilization')
        .setMin(0)
        .setMax(100);
      extraFilters['"average_30d"'] = new RangeFilterCategoryBuilder()
        .setLabel('30 Day Average Utilization')
        .setMin(0)
        .setMax(100);
    }
    if (!isDimensionsQueryProperlyLoaded) return extraFilters;
    return {
      ...dimensionsToFilterOptions(
        dimensionsQuery.data!,
        ANDROID_COLUMN_OVERRIDES,
      ),
      ...extraFilters,
    };
  }, [
    isDimensionsQueryProperlyLoaded,
    dimensionsQuery.data,
    showAvgUtilization,
  ]);

  const filterCategoryDatas = useFilters(filterOptions, {
    areFilterValuesLoading: !isDimensionsQueryProperlyLoaded,
  });

  const [isFiltering, setIsFiltering] = useState(false);

  const selectedOptions = useMemo<FilterState>(
    () => computeSelectedOptions(filterCategoryDatas.filterValues),
    [filterCategoryDatas.filterValues],
  );

  const onApplyFilter = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  }, [pagerCtx, setSearchParams, trackEvent]);

  const request = useMemo(
    () =>
      ListDevicesRequest.fromPartial({
        pageSize: getPageSize(pagerCtx, searchParams),
        pageToken: getPageToken(pagerCtx, searchParams),
        orderBy: orderByParam,
        filter: filterCategoryDatas.aip160(),
        platform: platform,
      }),
    [pagerCtx, searchParams, orderByParam, filterCategoryDatas],
  );

  const devicesQuery = useAndroidDevices(request);

  useEffect(() => {
    if (!devicesQuery.isFetching) {
      setIsFiltering(false);
    }
  }, [devicesQuery.isFetching]);

  const { devices = [] } = devicesQuery.data || {};

  const extraColumnIds = useMemo(() => {
    const ids = [
      'label-id',
      'label-run_target',
      'label-hostname',
      'fc_offline_since',
      'realm',
    ];
    if (showAvgUtilization) {
      ids.push('average_7d', 'average_30d');
    }
    return ids as (keyof typeof ANDROID_COLUMN_OVERRIDES)[];
  }, [showAvgUtilization]);
  const columnIds = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols = urlCols.length > 0 ? urlCols : ANDROID_DEFAULT_COLUMNS;

    return _.uniq([...requiredCols, ...extraColumnIds]);
  }, [searchParams, extraColumnIds]);

  const columnsList = useMemo(() => {
    return getAndroidColumns(columnIds);
  }, [columnIds]);

  const allDimensionColumns = useMemo(() => {
    const list: { id: string; label: string }[] = [];

    ANDROID_DEFAULT_COLUMNS.forEach((id) => list.push({ id, label: id }));
    extraColumnIds.forEach((id) => {
      let label = id;
      if (id === 'average_7d') label = '7 Day Average Utilization';
      if (id === 'average_30d') label = '30 Day Average Utilization';
      list.push({ id, label });
    });

    return _.uniqBy(list, 'id');
  }, [extraColumnIds]);

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (dimensionsQuery.isPending || devicesQuery.isPending) return;

    const missingParamsColumns = getWrongColumnsFromParams(
      searchParams,
      columnIds,
      ANDROID_DEFAULT_COLUMNS,
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
    devicesQuery.isPending,
    searchParams,
    setSearchParams,
  ]);

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    selectedOptions,
    columnsList: columnsList as unknown as FleetColumnDefExt[],
    orderByParam,
    localStorageKey: ANDROID_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: ANDROID_DEFAULT_COLUMNS,
    platform,
    isLoadingOptions: !isDimensionsQueryProperlyLoaded,
  });

  const {
    enrichedColumns,
    onRowSelectionChange,
    onSortingChange,
    rowSelection,
    sorting,
    columnVisibility,
    mrtColumnManager: {
      setColumnVisibility,
      onToggleColumn,
      resetDefaultColumns,
      selectOnlyColumn,
    },
    visibleColumnIds,
    goToPrevPage,
    goToNextPage,
    onRowsPerPageChange,
    columnSizing,
    onColumnSizingChange,
    resetColumnWidths,
  } = fleetMrtState;

  const visibleEnrichedColumns = useMemo(() => {
    return enrichedColumns.filter((col) => {
      const id = col.id ?? (col.accessorKey as string);
      return visibleColumnIds.includes(id);
    });
  }, [enrichedColumns, visibleColumnIds]);

  const meta = useMemo<FleetTableMeta<AndroidDevice>>(
    () => ({
      allDimensionColumns,
      visibleColumnIds,
      onToggleColumn,
      selectOnlyColumn,
      resetDefaultColumns,
      resetColumnWidths,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
      totalSize: devicesQuery.data?.totalSize,
      nextPageToken: devicesQuery.data?.nextPageToken,
    }),
    [
      allDimensionColumns,
      visibleColumnIds,
      onToggleColumn,
      selectOnlyColumn,
      resetDefaultColumns,
      resetColumnWidths,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
      devicesQuery.data?.totalSize,
      devicesQuery.data?.nextPageToken,
    ],
  );

  const tableOptions: MRT_TableOptions<AndroidDevice> & {
    filterValues?: Record<string, FilterCategory>;
  } = useMemo(
    () => ({
      columns: visibleEnrichedColumns as MRT_ColumnDef<AndroidDevice>[],
      data: devices as AndroidDevice[],
      displayColumnDefOptions: {
        'mrt-row-select': {
          size: 40,
        },
      },
      enableRowSelection: true,
      positionToolbarAlertBanner: 'none',
      getRowId: (row: AndroidDevice) => row.id,
      onRowSelectionChange,
      onSortingChange,
      enablePagination: false,
      filterValues: filterCategoryDatas.filterValues,
      meta,
      state: {
        rowSelection,
        sorting,
        columnVisibility,
        columnSizing,
        isLoading: devicesQuery.isPending && !devicesQuery.isPlaceholderData,
        showProgressBars: isFiltering || devicesQuery.isFetching,
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
      onColumnSizingChange,
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
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      visibleEnrichedColumns,
      devicesQuery,
      enrichedColumns,
      devices,
      onRowSelectionChange,
      onSortingChange,
      rowSelection,
      sorting,
      columnVisibility,
      devicesQuery.isPending,
      devicesQuery.isPlaceholderData,
      devicesQuery.isFetching,
      isFiltering,
      setColumnVisibility,
      visibleColumnIds,
      allDimensionColumns,
      onToggleColumn,
      resetDefaultColumns,
      resetColumnWidths,
      selectOnlyColumn,
      devicesQuery.data?.totalSize,
      devicesQuery.data?.nextPageToken,
      devicesQuery.data?.devices.length,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
      columnSizing,
      onColumnSizingChange,
      filterCategoryDatas.filterValues,
    ],
  );

  const table = useFCDataTable(tableOptions);

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications
        warnings={[
          ...(filterCategoryDatas.warnings || []),
          ...(warnings || []),
        ]}
      />
      <AndroidSummaryHeader
        aip160={filterCategoryDatas.aip160()}
        setFiltersBatch={filterCategoryDatas.setFiltersBatch}
        showAvgUtilization={showAvgUtilization}
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
        <FilterBar
          filterCategoryDatas={Object.values(
            filterCategoryDatas.filterValues || {},
          )}
          onApply={onApplyFilter}
          isLoading={
            dimensionsQuery.isPending ||
            filterCategoryDatas.filterValues === undefined
          }
          searchPlaceholder='Add a filter (e.g. "state:idle", "pool:default", or "device_id:123")'
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
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary key="fleet-device-list-page">
        <LoggedInBoundary>
          <AndroidDevicesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
