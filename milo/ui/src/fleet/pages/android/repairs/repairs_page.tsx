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
import { Alert, Button, Typography } from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_PaginationState,
  MRT_ColumnFiltersState,
} from 'material-react-table';
import { useMemo } from 'react';

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
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { GetFiltersResult } from '@/fleet/components/filter_dropdown/parser/parser';
import {
  StringListFilterCategory,
  StringListFilterCategoryBuilder,
} from '@/fleet/components/filters/string_list_filter';
import {
  useFilters,
  FilterCategory,
} from '@/fleet/components/filters/use_filters';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { BLANK_VALUE } from '@/fleet/constants/filters';
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
import { getErrorMessage } from '@/fleet/utils/errors';
import { computeSelectedOptions } from '@/fleet/utils/filters';
import {
  getFilterQueryString,
  parseOrderByParam,
} from '@/fleet/utils/search_param';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Platform,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getRepairsColumns, DEFAULT_COLUMNS } from './repairs_columns';
import { getPriorityIcon, getRow, type Row } from './repairs_columns.utils';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

export const RepairListPage = () => {
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useFleetConsoleClient();

  const repairMetricsFilterValues = useQuery({
    ...client.GetRepairMetricsDimensions.query({
      platform: Platform.ANDROID,
    }),
  });

  const loadedFilterOptions = useMemo(() => {
    if (!repairMetricsFilterValues.data) return {};

    const filters: Record<string, StringListFilterCategoryBuilder> = {};
    for (const [key, value] of Object.entries(
      repairMetricsFilterValues.data.dimensions,
    )) {
      const mappedKey =
        key === 'hostGroup'
          ? 'host_group'
          : key === 'labName'
            ? 'lab_name'
            : key;
      filters[mappedKey] = new StringListFilterCategoryBuilder()
        .setLabel(_.startCase(key))
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...(value.values || [])
            .filter((v) => v !== '' && v !== BLANK_VALUE)
            .map((v) => ({ label: v, value: v })),
        ]);
    }
    return filters;
  }, [repairMetricsFilterValues.data]);

  const filterCategoryDatas = useFilters(loadedFilterOptions, {
    areFilterValuesLoading: !repairMetricsFilterValues.data,
  });

  const selectedOptions = useMemo<GetFiltersResult>(() => {
    if (filterCategoryDatas.warnings.length > 0) {
      return {
        filters: undefined,
        error: new Error(filterCategoryDatas.warnings.join(', ')),
      };
    }
    return {
      filters: computeSelectedOptions(filterCategoryDatas.filterValues),
      error: undefined,
    };
  }, [filterCategoryDatas.filterValues, filterCategoryDatas.warnings]);

  const repairMetricsList = useQuery({
    ...client.ListRepairMetrics.query({
      platform: Platform.ANDROID,
      filter:
        filterCategoryDatas.warnings.length > 0
          ? ''
          : filterCategoryDatas.aip160(),
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
      orderBy: orderByParam,
    }),
    placeholderData: keepPreviousData,
  });

  const sorting = (searchParams.get(ORDER_BY_PARAM_KEY) ?? '')
    .split(', ')
    .map(parseOrderByParam)
    .filter((orderBy) => !!orderBy)
    .map((orderBy: { field: string; direction: OrderByDirection }) => ({
      id: orderBy.field,
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
    return getRepairsColumns(DEFAULT_COLUMNS).map((c) => {
      const id = getColumnId(c as MRT_ColumnDef<Row>);
      const builder = loadedFilterOptions[id];
      return {
        ...c,
        filterVariant: 'multi-select',
        filterSelectOptions:
          builder && builder.options
            ? builder.options.map((opt) => ({
                text: opt.label,
                value: String(opt.value),
              }))
            : [],
      };
    }) as MRT_ColumnDef<Row>[];
  }, [loadedFilterOptions]);

  const columnFilters = useMemo<MRT_ColumnFiltersState>(() => {
    return Object.entries(selectedOptions?.filters || {}).map(
      ([id, value]) => ({
        id,
        value,
      }),
    );
  }, [selectedOptions.filters]);

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
    filterValues: filterCategoryDatas.filterValues,
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
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
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
      <WarningNotifications warnings={filterCategoryDatas.warnings} />
      <Metrics
        filters={filterCategoryDatas.aip160()}
        pagerContext={pagerCtx}
        filterValues={filterCategoryDatas.filterValues}
      />
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
          onApply={() => {
            trackEvent('filter_changed', {
              componentName: 'device_list_filter',
            });
          }}
          isLoading={
            repairMetricsFilterValues.isPending ||
            filterCategoryDatas.filterValues === undefined
          }
          searchPlaceholder='Add a filter (e.g. "state:ready" or "priority:high")'
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

function Metrics({
  filters,
  pagerContext,
  filterValues,
}: {
  filters: string;
  pagerContext: PagerContext;
  filterValues: Record<string, FilterCategory> | undefined;
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
              handleClick={() => {
                const filter = filterValues?.['priority'];
                if (filter instanceof StringListFilterCategory) {
                  filter.setSelectedOptions(['BREACHED']);
                }
              }}
            />
            <SingleMetric
              name="Watch"
              value={countQuery.data?.watchRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.WATCH)}
              loading={countQuery.isPending}
              handleClick={() => {
                const filter = filterValues?.['priority'];
                if (filter instanceof StringListFilterCategory) {
                  filter.setSelectedOptions(['WATCH']);
                }
              }}
            />
            <SingleMetric
              name="Nice"
              value={countQuery.data?.niceRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.NICE)}
              loading={countQuery.isPending}
              handleClick={() => {
                const filter = filterValues?.['priority'];
                if (filter instanceof StringListFilterCategory) {
                  filter.setSelectedOptions(['NICE']);
                }
              }}
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
