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
import '@emotion/react';
import { ViewColumnOutlined } from '@mui/icons-material';
import { Button, colors, TablePagination } from '@mui/material';
import _ from 'lodash';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_TableInstance,
  MRT_TableOptions,
} from 'material-react-table';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  FleetColumnDefExt,
  useFleetMRTState,
} from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { FILTERS_PARAM_KEY } from '@/fleet/components/filter_dropdown/search_param_utils';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterState } from '@/fleet/components/filters/types';
import {
  FilterCategory,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
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
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AdminTasksAlert } from '../common/admin_tasks_alert';
import { dimensionsToFilterOptions } from '../common/helpers';
import { useDeviceDimensions } from '../common/use_device_dimensions';

import {
  ANDROID_COLUMN_OVERRIDES,
  getAndroidColumns,
  AndroidColumnDef,
} from './android_columns';
import { ANDROID_EXTRA_FILTERS } from './android_filters';
import { androidState } from './android_state';
import { AndroidSummaryHeader } from './android_summary_header';
import { dimensionsToFilterOptions as dimensionsToFilterOptionsAndroid } from './dimensions_to_filter_options';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

const platform = Platform.ANDROID;

const EXTRA_COLUMN_IDS = [
  'label-id',
  'label-run_target',
  'label-hostname',
  'fc_offline_since',
  'realm',
] satisfies (keyof typeof ANDROID_COLUMN_OVERRIDES)[];

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
    if (!isDimensionsQueryProperlyLoaded) return ANDROID_EXTRA_FILTERS;
    return {
      ...ANDROID_EXTRA_FILTERS,
      ...dimensionsToFilterOptions(
        dimensionsQuery.data!,
        ANDROID_COLUMN_OVERRIDES,
      ),
    };
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data]);

  const filterCategoryDatas = useFilters(filterOptions, {
    allowExtraKeys: !isDimensionsQueryProperlyLoaded,
  });

  const columnFiltersRef = useRef<{ id: string; value: unknown }[]>([]);
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

  const onColumnFiltersChangeOverride = useCallback(
    (newFilters: Record<string, string[]>) => {
      trackEvent('filter_changed', {
        componentName: 'device_list_filter',
      });
      if (!filterCategoryDatas.filterValues) return;

      const prevTableFilterKeys = columnFiltersRef.current.map((f) =>
        normalizeFilterKey(f.id),
      );

      for (const [key, category] of Object.entries(
        filterCategoryDatas.filterValues as Record<string, FilterCategory>,
      )) {
        const matchKey = normalizeFilterKey(key);

        const isInNewFilters =
          newFilters[matchKey] !== undefined || newFilters[key] !== undefined;
        const wasInTable =
          prevTableFilterKeys.includes(matchKey) ||
          prevTableFilterKeys.includes(key);

        if (!isInNewFilters && !wasInTable) {
          continue; // Leave FilterBar filters alone if not touched in table
        }

        const newValues = newFilters[matchKey] || newFilters[key] || [];

        let currentSelected: string[] = [];
        if (category instanceof StringListFilterCategory) {
          currentSelected = category.getSelectedOptions();
        }

        let isChanged = false;
        if (currentSelected.length !== newValues.length) {
          isChanged = true;
        } else {
          for (const k of currentSelected) {
            if (!newValues.includes(k) && !newValues.includes(`"${k}"`)) {
              isChanged = true;
            }
          }
        }

        if (isChanged) {
          if (category instanceof StringListFilterCategory) {
            category.setSelectedOptions(newValues, true);
          }
        }
      }

      const currentAIP160 = filterCategoryDatas.getAip160String();
      const prevAIP160 = searchParams.get(FILTERS_PARAM_KEY) || '';

      if (currentAIP160 !== prevAIP160) {
        setIsFiltering(true);
        setSearchParams((prev) => {
          let newParams = new URLSearchParams(prev);
          newParams.set(FILTERS_PARAM_KEY, currentAIP160);
          newParams = emptyPageTokenUpdater(pagerCtx)(newParams);
          return newParams;
        });
      }
    },
    [filterCategoryDatas, pagerCtx, searchParams, setSearchParams, trackEvent],
  );

  const request = useMemo(
    () =>
      ListDevicesRequest.fromPartial({
        pageSize: getPageSize(pagerCtx, searchParams),
        pageToken: getPageToken(pagerCtx, searchParams),
        orderBy: orderByParam,
        filter: filterCategoryDatas.parseError
          ? ''
          : filterCategoryDatas.aip160,
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

  const columnIds = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols = urlCols.length > 0 ? urlCols : ANDROID_DEFAULT_COLUMNS;

    return _.uniq([...requiredCols, ...EXTRA_COLUMN_IDS]);
  }, [searchParams]);

  const columnsList = useMemo(() => {
    const list = getAndroidColumns(columnIds);
    const filterValues = filterCategoryDatas.filterValues;
    if (!filterValues) return list;
    return list.map((col) => {
      const field = col.filterByField ?? col.orderByField ?? col.id ?? '';
      if (!field) return col;
      const category = (filterValues as Record<string, FilterCategory>)[field];
      if (category instanceof StringListFilterCategory) {
        return {
          ...col,
          filterSelectOptions: Object.values(category.getOptions()).map(
            (o) => ({
              label: o.label,
              value: o.key,
            }),
          ),
        };
      }
      return col;
    });
  }, [columnIds, filterCategoryDatas.filterValues]);

  const allDimensionColumns = useMemo(() => {
    const list: { id: string; label: string }[] = [];

    ANDROID_DEFAULT_COLUMNS.forEach((id) => list.push({ id, label: id }));
    EXTRA_COLUMN_IDS.forEach((id) => list.push({ id, label: id }));

    if (dimensionsQuery.data) {
      Object.keys(dimensionsQuery.data.baseDimensions).forEach((id) =>
        list.push({ id, label: id }),
      );
      Object.keys(dimensionsQuery.data.labels).forEach((id) =>
        list.push({ id, label: id }),
      );
    }
    return _.uniqBy(list, 'id');
  }, [dimensionsQuery.data]);

  const filterOptionsConfig = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return [];

    const columnsRecord = columnsList.reduce(
      (acc, col) => {
        const id = (col.accessorKey || col.id) as string;
        if (id) {
          acc[id] = col;
        }
        return acc;
      },
      {} as Record<string, AndroidColumnDef>,
    );

    const options = dimensionsToFilterOptionsAndroid(
      dimensionsQuery.data!,
      columnsRecord,
    );

    return [
      ...options,
      {
        label: 'State',
        type: 'string_list' as const,
        value: 'state',
        options: [
          { label: '(Blank)', value: '(Blank)' },
          ...Object.values(androidState).map((val) => ({
            label: val,
            value: val,
          })),
        ],
      },
    ];
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data, columnsList]);

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

  useEffect(() => {
    if (!isDimensionsQueryProperlyLoaded) return;
    if (!filterCategoryDatas.parseError) return;
    if (
      warnings.some((w) =>
        w.startsWith('There was an error parsing your filters:'),
      )
    )
      return;

    addWarning(
      `There was an error parsing your filters: ${filterCategoryDatas.parseError}`,
    );
  }, [
    addWarning,
    filterCategoryDatas,
    isDimensionsQueryProperlyLoaded,
    warnings,
  ]);

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    selectedOptions,
    filterOptionsConfig,
    columnsList: columnsList as unknown as FleetColumnDefExt[],
    orderByParam,
    localStorageKey: ANDROID_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: ANDROID_DEFAULT_COLUMNS,
    onColumnFiltersChangeOverride,
    platform,
    isLoadingOptions: !isDimensionsQueryProperlyLoaded,
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
    mrtColumnManager: {
      setColumnVisibility,
      onToggleColumn,
      resetDefaultColumns,
    },
    visibleColumnIds,
    goToPrevPage,
    goToNextPage,
    onRowsPerPageChange,
  } = fleetMrtState;

  const visibleEnrichedColumns = useMemo(() => {
    return enrichedColumns.filter((col) => {
      const id = col.id ?? (col.accessorKey as string);
      return visibleColumnIds.includes(id);
    });
  }, [enrichedColumns, visibleColumnIds]);

  // Sync ref to break circular dependency
  useEffect(() => {
    columnFiltersRef.current = columnFilters;
  }, [columnFilters]);

  const tableOptions: MRT_TableOptions<AndroidDevice> = useMemo(
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
      onColumnFiltersChange,
      enablePagination: false,
      state: {
        rowSelection,
        sorting,
        columnFilters,
        columnVisibility,
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
      muiTopToolbarProps: {
        sx: {
          '& [aria-label="Show/Hide filters"]': {
            display: 'none',
          },
        },
      },
      renderTopToolbarCustomActions: ({
        table,
      }: {
        table: MRT_TableInstance<AndroidDevice>;
      }) => (
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
            allColumns={allDimensionColumns}
            visibleColumns={visibleColumnIds}
            onToggleColumn={onToggleColumn}
            resetDefaultColumns={resetDefaultColumns}
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
        <div
          css={{ width: '100%', display: 'flex', justifyContent: 'flex-end' }}
        >
          <TablePagination
            component="div"
            count={
              (devicesQuery.data?.totalSize || 0) === 0 &&
              !devicesQuery.data?.nextPageToken &&
              !devicesQuery.data?.devices.length
                ? 0
                : -1
            }
            page={getCurrentPageIndex(pagerCtx)}
            rowsPerPage={getPageSize(pagerCtx, searchParams)}
            onPageChange={(_, page) => {
              const currentPage = getCurrentPageIndex(pagerCtx);
              const isPrevPage = page < currentPage;
              const isNextPage = page > currentPage;

              if (isPrevPage) {
                goToPrevPage();
              } else if (isNextPage) {
                goToNextPage(devicesQuery.data?.nextPageToken || '');
              }
            }}
            onRowsPerPageChange={(e) => {
              onRowsPerPageChange(Number(e.target.value));
            }}
            rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
            labelDisplayedRows={({ from, to }) => {
              const totalSize = devicesQuery.data?.totalSize;
              if (totalSize !== undefined && totalSize > 0) {
                return `${from}-${to} of ${totalSize}`;
              }
              return `${from}-${to} of ${
                devicesQuery.data?.nextPageToken ? `more than ${to}` : to
              }`;
            }}
            slotProps={{
              actions: {
                previousButtonProps: {
                  disabled: getCurrentPageIndex(pagerCtx) === 0,
                  onClick: goToPrevPage,
                },
                nextButtonProps: {
                  disabled:
                    !devicesQuery.data?.devices.length ||
                    !devicesQuery.data?.nextPageToken,
                  onClick: () =>
                    goToNextPage(devicesQuery.data?.nextPageToken || ''),
                },
              } as NonNullable<
                React.ComponentProps<typeof TablePagination>['slotProps']
              >['actions'],
            }}
          />
        </div>
      ),
    }),
    [
      visibleEnrichedColumns,
      enrichedColumns,
      devices,
      onRowSelectionChange,
      onSortingChange,
      onColumnFiltersChange,
      rowSelection,
      sorting,
      columnFilters,
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
      devicesQuery.data?.totalSize,
      devicesQuery.data?.nextPageToken,
      devicesQuery.data?.devices.length,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
    ],
  );

  const table = useFCDataTable(tableOptions);

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <AndroidSummaryHeader
        aip160={
          filterCategoryDatas.parseError ? '' : filterCategoryDatas.aip160
        }
        filters={filterCategoryDatas.filterValues}
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
          searchPlaceholder='Add a filter (e.g. "dut1" or "state:ready")'
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
