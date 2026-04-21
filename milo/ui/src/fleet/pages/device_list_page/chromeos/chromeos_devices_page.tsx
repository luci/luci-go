// Copyright 2024 The LUCI Authors.
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

import '@emotion/react';
import { useQuery } from '@tanstack/react-query';
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
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { RunAutorepair } from '@/fleet/components/actions/autorepair/run_autorepair';
import { CopyButton } from '@/fleet/components/actions/copy/copy_button';
import { CopySnackbar } from '@/fleet/components/actions/copy/copy_snackbar';
import { RunDeploy } from '@/fleet/components/actions/deploy/run_deploy';
import { RequestRepair } from '@/fleet/components/actions/request_repair/request_repair';
import { ExportButton_MRT } from '@/fleet/components/device_table/export_button_mrt';
import { useCurrentTasks } from '@/fleet/components/device_table/use_current_tasks';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { stripQuotes } from '@/fleet/components/fc_data_table/mrt_filter_menu_item_utils';
import {
  FleetTableMeta,
  useFleetTableMeta,
} from '@/fleet/components/fc_data_table/types';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  FleetColumnDefExt,
  useFleetMRTState,
} from '@/fleet/components/fc_data_table/use_fleet_mrt_state';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { FILTERS_PARAM_KEY } from '@/fleet/components/filter_dropdown/search_param_utils';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import {
  StringListFilterCategory,
  StringListFilterCategoryBuilder,
} from '@/fleet/components/filters/string_list_filter';
import { FilterState } from '@/fleet/components/filters/types';
import {
  FilterCategory,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { CHROMEOS_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { CHROMEOS_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useDevices } from '@/fleet/hooks/use_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import {
  extractDutId,
  extractDutLabel,
  extractDutLabels,
  extractDutState,
} from '@/fleet/utils/devices';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountDevicesRequest,
  Device,
  ListDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AdminTasksAlert } from '../common/admin_tasks_alert';
import { dimensionsToFilterOptions } from '../common/helpers';
import { useDeviceDimensions } from '../common/use_device_dimensions';

import {
  CHROMEOS_COLUMN_OVERRIDES,
  getChromeOSColumns,
  ChromeOSDevice,
  EXTRA_COLUMN_IDS,
} from './chromeos_columns';
import { ChromeOSSummaryHeader } from './chromeos_summary_header';
import { dutState } from './dut_state';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

const platform = Platform.CHROMEOS;

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
        const matchKey = key;
        const unquotedSelected = selected.map((k) =>
          typeof k === 'string' ? stripQuotes(k) : k,
        );
        filters[matchKey] = unquotedSelected;
      }
    }
  }
  return { filters, error: undefined };
};

// TODO(b/479452001): Extract this to a shared utility file when another page needs it.
const syncFilterCategory = (
  key: string,
  category: FilterCategory,
  newFilters: Record<string, string[]>,
  prevTableFilterKeys: string[],
) => {
  const matchKey = normalizeFilterKey(key);

  const isInNewFilters =
    newFilters[matchKey] !== undefined || newFilters[key] !== undefined;
  const wasInTable =
    prevTableFilterKeys.includes(matchKey) || prevTableFilterKeys.includes(key);

  if (!isInNewFilters && !wasInTable) {
    return;
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
};

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
    state: extractDutState(row as Device),
    pool: extractDutLabel('label-pool', row as Device),
    board: extractDutLabel('label-board', row as Device),
    model: extractDutLabel('label-model', row as Device),
    namespace: extractDutLabels('ufs_namespace', row as Device),
  }));
  const meta = useFleetTableMeta(table);

  if (selectedRows.length === 0) {
    return <ExportButton_MRT table={table} selectedRowIds={[]} />;
  }

  return (
    <>
      <RunAutorepair selectedDuts={selectedDuts} />
      <RunDeploy selectedDuts={selectedDuts} />
      <CopyButton onClick={() => meta.handleCopy?.(table)} />
      <RequestRepair selectedDuts={selectedDuts} />
      <ExportButton_MRT
        table={table}
        selectedRowIds={selectedRows.map((row) => `${row.id}`)}
      />
    </>
  );
};

export const ChromeOSDevicesPage = () => {
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useFleetConsoleClient();
  const dimensionsQuery = useDeviceDimensions({ platform });

  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.labels;

  const filterOptions = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return {};
    return dimensionsToFilterOptions(
      dimensionsQuery.data!,
      CHROMEOS_COLUMN_OVERRIDES,
    );
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data]);

  const filterCategoryDatas = useFilters(filterOptions, {
    allowExtraKeys: !isDimensionsQueryProperlyLoaded,
  });

  const columnFiltersRef = useRef<{ id: string; value: unknown }[]>([]);
  const [isFiltering, setIsFiltering] = useState(false);
  const [showCopySuccess, setShowCopySuccess] = useState(false);

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
        syncFilterCategory(key, category, newFilters, prevTableFilterKeys);
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

  const filterOptionsConfig = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return [];

    const options = Object.entries(filterOptions).map(([key, builder]) => {
      const b = builder as StringListFilterCategoryBuilder;
      return {
        label: b.label || key,
        type: 'string_list' as const,
        value: key,
        options: b.options || [],
      };
    });

    return [
      ...options,
      {
        label: 'Dut State',
        type: 'string_list' as const,
        value: 'labels.dut_state',
        options: Object.values(dutState).map((val) => ({
          label: val,
          value: val,
        })),
      },
    ];
  }, [isDimensionsQueryProperlyLoaded, filterOptions]);

  const countQuery = useQuery({
    ...client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: filterCategoryDatas.parseError
          ? ''
          : filterCategoryDatas.aip160,
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
        filter: filterCategoryDatas.parseError
          ? ''
          : filterCategoryDatas.aip160,
        platform: Platform.CHROMEOS,
      }),
    [pagerCtx, searchParams, orderByParam, filterCategoryDatas],
  );

  const devicesQuery = useDevices(request);

  useEffect(() => {
    if (!devicesQuery.isFetching) {
      setIsFiltering(false);
    }
  }, [devicesQuery.isFetching]);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};

  const columnIds = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols =
      urlCols.length > 0 ? urlCols : CHROMEOS_DEFAULT_COLUMNS;

    return _.uniq([...requiredCols, ...EXTRA_COLUMN_IDS]);
  }, [searchParams]);

  const columnsList = useMemo(() => {
    return getChromeOSColumns(columnIds);
  }, [columnIds]);

  const allDimensionColumns = useMemo(() => {
    const list: { id: string; label: string }[] = [];

    CHROMEOS_DEFAULT_COLUMNS.forEach((id) => list.push({ id, label: id }));
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

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (dimensionsQuery.isPending || devicesQuery.isPending) return;

    const missingParamsColumns = getWrongColumnsFromParams(
      searchParams,
      columnIds,
      CHROMEOS_DEFAULT_COLUMNS,
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

  const currentTasks = useCurrentTasks(devices);

  const fleetMrtState = useFleetMRTState({
    setSearchParams,
    pagerCtx,
    selectedOptions,
    filterOptionsConfig,
    columnsList: columnsList as unknown as FleetColumnDefExt[],
    orderByParam,
    localStorageKey: CHROMEOS_DEVICES_LOCAL_STORAGE_KEY,
    defaultColumnIds: CHROMEOS_DEFAULT_COLUMNS,
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

  const rows: ChromeOSDevice[] = useMemo(() => {
    const baseRows = currentTasks.isPending
      ? devices.map((d) => ({
          ...d,
          current_task: undefined,
        }))
      : devices.map((d) => ({
          ...d,
          current_task: currentTasks.map.get(extractDutId(d)) || '',
        }));

    return baseRows;
  }, [devices, currentTasks.map, currentTasks.isPending]);

  useEffect(() => {
    columnFiltersRef.current = columnFilters;
  }, [columnFilters]);

  // TODO(b/479452001): Extract this to a hook / shared between chromeos and other pages.
  const handleCopy = useCallback((table: MRT_TableInstance<ChromeOSDevice>) => {
    const selectedRows = table
      .getSelectedRowModel()
      .rows.map((r) => r.original);

    const visibleColumns = table.getVisibleLeafColumns();

    const headers = visibleColumns
      .map((c) => c.columnDef.header ?? c.id)
      .join('\t');

    const body = selectedRows
      .map((row) => {
        return visibleColumns
          .map((c) => {
            const colDef = c.columnDef;
            const val =
              'accessorFn' in colDef && typeof colDef.accessorFn === 'function'
                ? colDef.accessorFn(row, 0)
                : row[c.id as keyof ChromeOSDevice];
            return String(val ?? '');
          })
          .join('\t');
      })
      .join('\n');

    const finalString = `${headers}\n${body}`;
    navigator.clipboard.writeText(finalString);
    setShowCopySuccess(true);
  }, []);

  const meta = useMemo<FleetTableMeta<ChromeOSDevice>>(
    () => ({
      handleCopy,
      allDimensionColumns,
      visibleColumnIds,
      onToggleColumn,
      resetDefaultColumns,
      devicesQuery,
      nextPageToken,
      devices,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
      countQuery,
      totalSize: countQuery?.data?.total,
    }),
    [
      handleCopy,
      allDimensionColumns,
      visibleColumnIds,
      onToggleColumn,
      resetDefaultColumns,
      devicesQuery,
      nextPageToken,
      devices,
      pagerCtx,
      searchParams,
      goToPrevPage,
      goToNextPage,
      onRowsPerPageChange,
      countQuery,
    ],
  );

  const tableOptions: MRT_TableOptions<ChromeOSDevice> = useMemo(
    () => ({
      columns: fleetMrtState.enrichedColumns as MRT_ColumnDef<ChromeOSDevice>[],
      data: rows,
      displayColumnDefOptions: {
        'mrt-row-select': {
          size: 40,
        },
      },
      enableRowSelection: true,
      positionToolbarAlertBanner: 'none',
      getRowId: (row: ChromeOSDevice) => row.id,
      onRowSelectionChange,
      onSortingChange,
      onColumnFiltersChange,
      enablePagination: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      meta,
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
      renderTopToolbarCustomActions: ({ table }) => (
        <FleetTopToolbar table={table}>
          <ChromeOSActions table={table} />
        </FleetTopToolbar>
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar table={table} />
      ),
    }),
    [
      meta,
      enrichedColumns,
      rows,
      onRowSelectionChange,
      onSortingChange,
      onColumnFiltersChange,
      rowSelection,
      sorting,
      columnFilters,
      fleetMrtState.enrichedColumns,
      columnVisibility,
      devicesQuery,
      isFiltering,
      setColumnVisibility,
    ],
  );

  const table = useFCDataTable(tableOptions);

  return (
    <div
      css={{
        margin: '24px',
        paddingBottom: '40px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <ChromeOSSummaryHeader
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
        {/* TODO(b/479452001): Extract some DeviceTable back / split this file into smaller pieces in a followup CL. */}
        <MaterialReactTable table={table} />
      </div>
      <CopySnackbar
        open={showCopySuccess}
        onClose={() => setShowCopySuccess(false)}
      />
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary key="fleet-device-list-page">
        <LoggedInBoundary>
          <ChromeOSDevicesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
