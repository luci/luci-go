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
import { MaterialReactTable, MRT_TableOptions } from 'material-react-table';
import { useCallback, useEffect, useMemo } from 'react';

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
import { useFleetMRTState } from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getAndroidColumns } from '@/fleet/pages/device_list_page/android/android_columns';
import {
  ANDROID_COLUMN_OVERRIDES,
  AndroidColumnDef,
} from '@/fleet/pages/device_list_page/android/android_fields';
import { ANDROID_EXTRA_FILTERS } from '@/fleet/pages/device_list_page/android/android_filters';
import { AndroidSummaryHeader } from '@/fleet/pages/device_list_page/android/android_summary_header';
import { AdminTasksAlert } from '@/fleet/pages/device_list_page/common/admin_tasks_alert';
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

  const onApplyFilter = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  }, [pagerCtx, setSearchParams, trackEvent]);

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

    const filters = extraFilters as Record<
      string,
      FilterCategoryBuilder<FilterCategory>
    >;
    const data = dimensionsQuery.data!;
    const baseKeys = Object.keys(data.baseDimensions);

    for (const [key, value] of [
      ...Object.entries(data.baseDimensions),
      ...Object.entries(data.labels),
    ]) {
      if (value.values.length === 0) continue;

      const isBase = baseKeys.includes(key);
      const filterKey = isBase ? `"${key}"` : `labels."${key}"`;

      if (filters[filterKey]) continue;

      const override = ANDROID_COLUMN_OVERRIDES[key];
      const label = override?.header || key;

      filters[filterKey] = new StringListFilterCategoryBuilder()
        .setLabel(label)
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...value.values.map((v) => ({ label: v, value: v })),
        ]);
    }

    return filters;
  }, [
    isDimensionsQueryProperlyLoaded,
    dimensionsQuery.data,
    showAvgUtilization,
  ]);

  const filterCategoryDatas = useFilters(filterOptions, {
    areFilterValuesLoading: !isDimensionsQueryProperlyLoaded,
    onFilterChange: onApplyFilter,
  });

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

  const availableColumns = useMemo(() => {
    const list: { id: string; label: string }[] = [];

    ANDROID_DEFAULT_COLUMNS.forEach((id) => list.push({ id, label: id }));
    extraColumnIds.forEach((id) => {
      let label = id;
      if (id === 'average_7d') label = '7 Day Average Utilization';
      if (id === 'average_30d') label = '30 Day Average Utilization';
      list.push({ id, label });
    });

    if (dimensionsQuery.data) {
      Object.keys(dimensionsQuery.data.baseDimensions).forEach((id) =>
        list.push({ id, label: id }),
      );
      Object.keys(dimensionsQuery.data.labels).forEach((id) =>
        list.push({ id, label: id }),
      );
    }

    return _.uniqBy(list, 'id');
  }, [extraColumnIds, dimensionsQuery.data]);

  const allColumns = useMemo(() => {
    return getAndroidColumns(availableColumns.map((c) => c.id));
  }, [availableColumns]);

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

  const fleetMrtState = useFleetMRTState<AndroidColumnDef>({
    setSearchParams,
    pagerCtx,
    filterValues: filterCategoryDatas.filterValues,
    visibleColumns: allColumns,
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

  const meta = useMemo<FleetTableMeta<AndroidDevice>>(
    () => ({
      availableColumns,
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
      availableColumns,
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
      columns: enrichedColumns,
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
    [
      devices,
      onRowSelectionChange,
      onSortingChange,
      filterCategoryDatas.filterValues,
      meta,
      rowSelection,
      sorting,
      columnVisibility,
      columnSizing,
      devicesQuery.isPending,
      devicesQuery.isPlaceholderData,
      devicesQuery.isFetching,
      enrichedColumns,
      setColumnVisibility,
      onColumnSizingChange,
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
