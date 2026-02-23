// Copyright 2025 The LUCI Authors.
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

import DoneIcon from '@mui/icons-material/Done';
import ErrorIcon from '@mui/icons-material/Error';
import ViewColumnOutlined from '@mui/icons-material/ViewColumnOutlined';
import { Alert, Button, Chip, Typography } from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_PaginationState,
  MRT_ColumnFiltersState,
  MRT_Updater,
} from 'material-react-table';
import { useEffect, useMemo, useCallback } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  nextPageTokenUpdater,
  PagerContext,
  pageSizeUpdater,
  prevPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import {
  getColumnId,
  useMRTColumnManagement,
} from '@/fleet/components/columns/use_mrt_column_management';
import { DeviceListFilterBar } from '@/fleet/components/device_table/device_list_filter_bar';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import {
  filtersUpdater,
  getFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import {
  ANDROID_PLATFORM,
  generateDeviceListURL,
} from '@/fleet/constants/paths';
import {
  ORDER_BY_PARAM_KEY,
  OrderByDirection,
  useOrderByParam,
} from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  getFilterQueryString,
  parseOrderByParam,
} from '@/fleet/utils/search_param';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Platform,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { COLUMNS, getPriorityIcon, getRow, Row } from './repairs_columns';
import { dimensionsToFilterOptions } from './repairs_page_utils';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

export const RepairListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const selectedOptions = useMemo(
    () => getFilters(searchParams),
    [searchParams],
  );

  const onSelectedOptionsChange = useCallback(
    (newSelectedOptions: SelectedOptions) => {
      setSearchParams(filtersUpdater(newSelectedOptions));

      // Clear out all the page tokens when the filter changes.
      // An AIP-158 page token is only valid for the filter
      // option that generated it.
      setSearchParams(emptyPageTokenUpdater(pagerCtx));
    },
    [pagerCtx, setSearchParams],
  );

  const stringifiedSelectedOptions = selectedOptions.error
    ? ''
    : stringifyFilters(selectedOptions.filters);

  const client = useFleetConsoleClient();

  const repairMetricsList = useQuery({
    ...client.ListRepairMetrics.query({
      platform: Platform.ANDROID,
      filter: stringifiedSelectedOptions,
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
      orderBy: orderByParam,
    }),
    placeholderData: keepPreviousData,
  });

  const repairMetricsFilterValues = useQuery({
    ...client.GetRepairMetricsDimensions.query({
      platform: Platform.ANDROID,
    }),
  });

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (selectedOptions.error) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) => !COLUMNS[_.snakeCase(filterKey) as keyof typeof COLUMNS],
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
  }, [addWarning, selectedOptions, setSearchParams]);

  useEffect(() => {
    if (!selectedOptions.error) return;
    addWarning('Invalid filters');
    setSearchParams(filtersUpdater({}));
  }, [addWarning, selectedOptions.error, setSearchParams]);

  const sorting = (searchParams.get(ORDER_BY_PARAM_KEY) ?? '')
    .split(', ')
    .map(parseOrderByParam)
    .filter((orderBy) => !!orderBy)
    .map((orderBy: { field: string; direction: OrderByDirection }) => ({
      id: COLUMNS[orderBy.field as keyof typeof COLUMNS].accessorKey,
      desc: orderBy.direction === OrderByDirection.DESC,
    }));
  const pagination = useMemo<MRT_PaginationState>(
    () => ({
      pageIndex: getCurrentPageIndex(pagerCtx),
      pageSize: getPageSize(pagerCtx, searchParams),
    }),
    [pagerCtx, searchParams],
  );

  const columns = useMemo(() => {
    const filterCategories = repairMetricsFilterValues.data
      ? dimensionsToFilterOptions(repairMetricsFilterValues.data)
      : [];

    return Object.values(COLUMNS).map((c) => {
      const id = getColumnId(c as MRT_ColumnDef<Row>);
      const category = filterCategories.find((cat) => cat.value === id);
      return {
        ...c,
        filterVariant: 'multi-select',
        filterSelectOptions:
          category && 'options' in category ? category.options : [],
      };
    }) as MRT_ColumnDef<Row>[];
  }, [repairMetricsFilterValues.data]);

  const columnFilters = useMemo<MRT_ColumnFiltersState>(() => {
    return Object.entries(selectedOptions?.filters || {}).map(
      ([id, value]) => ({
        id,
        value,
      }),
    );
  }, [selectedOptions.filters]);

  const onColumnFiltersChange = useCallback(
    (updater: MRT_Updater<MRT_ColumnFiltersState>) => {
      const newFilters =
        typeof updater === 'function' ? updater(columnFilters) : updater;
      const newSelectedOptions = Object.fromEntries(
        newFilters.map((f: { id: string; value: unknown }) => [f.id, f.value]),
      ) as SelectedOptions;

      onSelectedOptionsChange(newSelectedOptions);
    },
    [columnFilters, onSelectedOptionsChange],
  );

  const defaultColumnIds = useMemo(
    () => columns.map((c) => getColumnId(c as MRT_ColumnDef<Row>)),
    [columns],
  );

  const highlightedColumnIds = useMemo(
    () => Object.keys(selectedOptions.filters || {}),
    [selectedOptions.filters],
  );

  const mrtColumnManager = useMRTColumnManagement({
    columns,
    defaultColumnIds,
    localStorageKey: 'fleet-console-repairs-columns',
    highlightedColumnIds,
  });

  const table = useFCDataTable({
    columns: mrtColumnManager.columns,
    enableColumnActions: true,
    enableColumnFilters: false,
    manualFiltering: true,
    positionToolbarAlertBanner: 'none',
    enableRowSelection: true,
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
          allColumns={mrtColumnManager.allColumns}
          visibleColumns={mrtColumnManager.visibleColumnIds}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          renderTrigger={({ onClick }, ref) => (
            <Button
              ref={ref}
              startIcon={<ViewColumnOutlined sx={{ fontSize: '26px' }} />}
              onClick={onClick}
              color="inherit"
              sx={{ color: colors.grey[600], height: '40px' }}
            >
              Columns
            </Button>
          )}
        />
      </div>
    ),
    data: repairMetricsList.data?.repairMetrics.map(getRow) ?? [],
    getRowId: (row) => row.lab_name + row.host_group + row.run_target,
    state: {
      sorting,
      columnFilters,
      columnVisibility: mrtColumnManager.columnVisibility,
      showProgressBars:
        repairMetricsList.isPending || repairMetricsList.isPlaceholderData,
      showAlertBanner: repairMetricsList.isError,
      pagination: pagination,
    },
    onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
    onColumnFiltersChange,
    muiPaginationProps: {
      rowsPerPageOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    },
    muiToolbarAlertBannerProps: repairMetricsList.isError
      ? {
          color: 'error',
          children: getErrorMessage(
            repairMetricsList.error,
            'get repair metrics',
          ),
        }
      : undefined,

    // Sorting
    manualSorting: true,
    onSortingChange: (updater) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      updateOrderByParam(
        newSorting
          .map((sort) => {
            if (sort.desc) return `${sort.id} desc`;
            return sort.id;
          })
          .join(', '),
      );

      setSearchParams(
        (prev: URLSearchParams) => emptyPageTokenUpdater(pagerCtx)(prev),
        {
          replace: true,
        },
      );
    },

    // Pagination
    rowCount: repairMetricsList.data?.totalSize ?? 0,
    manualPagination: true,
    onPaginationChange: (updater) => {
      const oldPagination = {
        pageIndex: getCurrentPageIndex(pagerCtx),
        pageSize: getPageSize(pagerCtx, searchParams),
      };

      const newPagination =
        typeof updater === 'function' ? updater(oldPagination) : updater;

      setSearchParams(pageSizeUpdater(pagerCtx, newPagination.pageSize));
      const currentPage = getCurrentPageIndex(pagerCtx);
      const isPrevPage = newPagination.pageIndex < currentPage;
      const isNextPage = newPagination.pageIndex > currentPage;

      setSearchParams((prev: URLSearchParams) => {
        let next = pageSizeUpdater(pagerCtx, newPagination.pageSize)(prev);
        if (isPrevPage) {
          next = prevPageTokenUpdater(pagerCtx)(next);
        } else if (isNextPage) {
          next = nextPageTokenUpdater(
            pagerCtx,
            repairMetricsList.data?.nextPageToken ?? '',
          )(next);
        }
        return next;
      });
    },
  });

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <Metrics filters={stringifiedSelectedOptions} pagerContext={pagerCtx} />
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
            filterOptions={
              repairMetricsFilterValues.data
                ? dimensionsToFilterOptions(repairMetricsFilterValues.data)
                : []
            }
            selectedOptions={selectedOptions.filters}
            onSelectedOptionsChange={onSelectedOptionsChange}
            isLoading={
              repairMetricsFilterValues.isPending ||
              repairMetricsFilterValues.isPlaceholderData
            }
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

function Metrics({
  filters,
  pagerContext,
}: {
  filters: string;
  pagerContext: PagerContext;
}) {
  const client = useFleetConsoleClient();
  const [searchParams, _] = useSyncedSearchParams();
  const countQuery = useQuery({
    ...client.CountRepairMetrics.query({
      platform: Platform.ANDROID,
      filter: filters,
    }),
  });

  const crossPageSearchParams = new URLSearchParams(searchParams);
  crossPageSearchParams.delete(ORDER_BY_PARAM_KEY);
  crossPageSearchParams.delete('c');

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
      );
    }

    return (
      <div
        css={{
          display: 'flex',
        }}
      >
        <div
          css={{
            display: 'flex',
            flexDirection: 'column',
            flexGrow: 1,
            borderRight: `1px solid ${colors.grey[300]}`,
            marginRight: 15,
            paddingRight: 15,
          }}
        >
          <Typography variant="subhead1">Hosts</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-between',
              marginTop: 5,
              flexWrap: 'wrap',
            }}
          >
            <SingleMetric
              name="Total Hosts"
              value={countQuery.data?.totalHosts}
              loading={countQuery.isPending}
              filterUrl={
                generateDeviceListURL(ANDROID_PLATFORM) +
                getFilterQueryString(
                  { fc_machine_type: ['host'] },
                  crossPageSearchParams,
                  pagerContext,
                )
              }
            />
            <SingleMetric
              name="Distinct Hosts Offline"
              value={countQuery.data?.offlineHosts}
              total={countQuery.data?.totalHosts}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={
                generateDeviceListURL(ANDROID_PLATFORM) +
                getFilterQueryString(
                  {
                    fc_machine_type: ['host'],
                    state: ['LAB_MISSING'],
                  },
                  crossPageSearchParams,
                  pagerContext,
                )
              }
            />
            {/* needed to left align content while keeping the correct right spacing*/}
            <div />
          </div>
        </div>
        <div
          css={{
            display: 'flex',
            flexDirection: 'column',
            flexGrow: 1,
            borderRight: `1px solid ${colors.grey[300]}`,
            marginRight: 15,
            paddingRight: 15,
          }}
        >
          <Typography variant="subhead1">Devices</Typography>
          <div
            css={{
              display: 'flex',
              marginTop: 5,
              marginLeft: 8,
              flexWrap: 'wrap',
              justifyContent: 'space-between',
            }}
          >
            <SingleMetric
              name="Total"
              value={countQuery.data?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={
                generateDeviceListURL(ANDROID_PLATFORM) +
                getFilterQueryString(
                  { fc_machine_type: ['device'] },
                  crossPageSearchParams,
                  pagerContext,
                )
              }
            />
            <SingleMetric
              name="Distinct Devices Online"
              value={
                countQuery.data?.totalDevices
                  ? countQuery.data?.totalDevices -
                    (countQuery.data?.offlineDevices || 0)
                  : undefined
              }
              total={countQuery.data?.totalDevices}
              Icon={<DoneIcon sx={{ color: colors.green[600] }} />}
              loading={countQuery.isPending}
              filterUrl={
                generateDeviceListURL(ANDROID_PLATFORM) +
                getFilterQueryString(
                  {
                    fc_machine_type: ['device'],
                    fc_is_offline: ['false'],
                  },
                  crossPageSearchParams,
                  pagerContext,
                )
              }
            />
            <SingleMetric
              name="Distinct Devices Offline"
              value={countQuery.data?.offlineDevices}
              total={countQuery.data?.totalDevices}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={
                generateDeviceListURL(ANDROID_PLATFORM) +
                getFilterQueryString(
                  {
                    fc_machine_type: ['device'],
                    fc_is_offline: ['true'],
                  },
                  crossPageSearchParams,
                  pagerContext,
                )
              }
            />
            {/* needed to left align content while keeping the correct right spacing*/}
            <div />
          </div>
        </div>
        <div
          css={{
            display: 'flex',
            flexDirection: 'column',
            flexGrow: 2,
          }}
        >
          <div css={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <Typography variant="subhead1">Repair groups</Typography>
            <InfoTooltip>
              A repair group is defined by aggregating lab name + host group +
              run target and corresponds to one of the rows of the table below
            </InfoTooltip>
          </div>
          <div
            css={{
              display: 'flex',
              marginTop: 5,
              marginLeft: 8,
              flexWrap: 'wrap',
              justifyContent: 'space-between',
            }}
          >
            <SingleMetric
              name="Total"
              value={countQuery.data?.totalRepairGroup}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Breached"
              value={countQuery.data?.breachedRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.BREACHED)}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                { priority: ['BREACHED'] },
                searchParams,
              )}
            />
            <SingleMetric
              name="Watch"
              value={countQuery.data?.watchRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.WATCH)}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                { priority: ['WATCH'] },
                searchParams,
              )}
            />
            <SingleMetric
              name="Nice"
              value={countQuery.data?.niceRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.NICE)}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                { priority: ['NICE'] },
                searchParams,
              )}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <div
      css={{
        padding: '16px 21px',
        gap: 28,
        border: `1px solid ${colors.grey[300]}`,
        borderRadius: 4,
      }}
    >
      <Typography variant="h4">Repair metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </div>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-repairs">
      <FleetHelmet pageTitle="Repairs" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-repairs"
      >
        <LoggedInBoundary>
          <RepairListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
