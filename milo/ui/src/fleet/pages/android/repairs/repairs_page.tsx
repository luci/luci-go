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
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import ViewColumnOutlined from '@mui/icons-material/ViewColumnOutlined';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Button, Chip, Divider, Typography } from '@mui/material';
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
import { Link } from 'react-router';

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
import { RunTargetColumnHeader } from '@/fleet/pages/device_list_page/android/run_target_column_header';
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
  RepairMetric,
  RepairMetric_Priority,
  repairMetric_PriorityToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { dimensionsToFilterOptions } from './repairs_page_utils';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

const getPriorityIcon = (priority: RepairMetric_Priority) => {
  switch (priority) {
    case RepairMetric_Priority.NICE:
      return <DoneIcon sx={{ color: colors.green[400], width: '20px' }} />;
    case RepairMetric_Priority.MISSING_DATA:
    case RepairMetric_Priority.DEVICES_REMOVED:
    case RepairMetric_Priority.WATCH:
      return <WarningIcon sx={{ color: colors.yellow[900], width: '20px' }} />;
    case RepairMetric_Priority.BREACHED:
      return <ErrorIcon sx={{ color: colors.red[500], width: '20px' }} />;
  }
};

// mapping to snake case is needed to send the right sort "by" to the backend
const getRow = (rm: RepairMetric) => ({
  id: rm.labName + rm.hostGroup + rm.runTarget,
  priority: rm.priority,
  lab_name: rm.labName,
  host_group: rm.hostGroup,
  run_target: rm.runTarget,
  minimum_repairs: rm.minimumRepairs,
  devices_offline_ratio: `${rm.devicesOffline} / ${rm.totalDevices}`,
  devices_offline_percentage: (
    rm.devicesOffline / rm.totalDevices
  ).toLocaleString('en-GB', {
    style: 'percent',
    minimumFractionDigits: 1,
  }),
  peak_usage: rm.peakUsage,
  total_devices: rm.totalDevices,
});

type Row = ReturnType<typeof getRow>;

const COLUMNS = {
  priority: {
    accessorKey: 'priority',
    header: 'Priority',
    size: 80,
    Header: () => {
      return (
        <>
          <div
            css={{
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <Typography
              variant="subhead2"
              sx={{ overflowX: 'hidden', textOverflow: 'ellipsis' }}
            >
              Priority
            </Typography>
            <InfoTooltip infoCss={{ marginLeft: '10px' }}>
              <div
                css={{
                  display: 'grid',
                  gridTemplateColumns: 'auto auto',
                  rowGap: '4px',
                }}
              >
                <div>
                  <div
                    css={{
                      gap: '4px',
                      paddingRight: 8, // columnGap doesn't work because of the line divider
                      display: 'flex',
                      alignItems: 'center',
                    }}
                  >
                    {getPriorityIcon(RepairMetric_Priority.BREACHED)}
                    <Typography variant="body2">BREACHED</Typography>
                  </div>
                </div>
                <Typography variant="body2">
                  The minimum number of repairs needed to meet SLOs SLO-2 is
                  considered breached when the Offline Ratio is above 10% and
                  allocated Ratio is above 80%
                </Typography>
                <Divider
                  css={{
                    backgroundColor: 'transparent',
                    gridColumn: '1 / span 99',
                  }}
                />
                <div>
                  <div
                    css={{
                      gap: '4px',
                      display: 'flex',
                      alignItems: 'center',
                    }}
                  >
                    {getPriorityIcon(RepairMetric_Priority.WATCH)}
                    <Typography variant="body2">WATCH</Typography>
                  </div>
                </div>
                <Typography variant="body2">
                  SLO-2 is considered at risk when the Offline Ratio is above 8%
                  and this time we check for Peak Utilization to be above 80%
                </Typography>
                <Divider
                  css={{
                    backgroundColor: 'transparent',
                    gridColumn: '1 / span 99',
                  }}
                />
                <div>
                  <div
                    css={{
                      gap: '4px',
                      display: 'flex',
                      alignContent: 'center',
                    }}
                  >
                    {getPriorityIcon(RepairMetric_Priority.NICE)}
                    {/*
                    the `tick` icon is very visually bottom heavy so to look aligned
                    we need to move the text a bit further down
                  */}
                    <Typography variant="body2" css={{ paddingTop: 3 }}>
                      NICE
                    </Typography>
                  </div>
                </div>
                <Typography variant="body2" css={{ paddingTop: 4 }}>
                  Everything else is considered nice
                </Typography>
              </div>
            </InfoTooltip>
          </div>
        </>
      );
    },
    Cell: (x) => {
      return (
        <div
          css={{
            gap: '4px',
            display: 'flex',
            alignItems: 'center',
            height: '100%',
          }}
        >
          {getPriorityIcon(x.cell.getValue())}
          <Typography variant="body2" noWrap={true}>
            {repairMetric_PriorityToJSON(x.cell.getValue())}
          </Typography>
        </div>
      );
    },
  },
  lab_name: {
    accessorKey: 'lab_name',
    header: 'Lab Name',
    size: 60,
  },
  host_group: {
    accessorKey: 'host_group',
    header: 'Host Group',
    size: 120,
  },
  run_target: {
    accessorKey: 'run_target',
    header: 'Run Target',
    size: 60,
    Header: () => <RunTargetColumnHeader />,
  },
  minimum_repairs: {
    accessorKey: 'minimum_repairs',
    header: 'Minimum Repairs',
    sortDescFirst: true,
    size: 80,
    Header: () => {
      return (
        <div
          css={{
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <Typography
            variant="subhead2"
            sx={{ overflowX: 'hidden', textOverflow: 'ellipsis' }}
          >
            Minimum Repairs
          </Typography>
          <InfoTooltip infoCss={{ marginLeft: '10px' }}>
            The minimum number of repairs needed to meet SLOs
          </InfoTooltip>
        </div>
      );
    },
  },
  devices_offline_percentage: {
    accessorKey: 'devices_offline_percentage',
    header: 'Devices Offline %',
    sortDescFirst: true,
    size: 80,
    Header: () => {
      return (
        <Typography
          variant="subhead2"
          css={{
            fontWeight: 500,
            textWrap: 'wrap',
            overflowX: 'hidden',
            textOverflow: 'ellipsis',
          }}
        >
          Devices Offline&nbsp;%
        </Typography>
      );
    },
  },
  devices_offline_ratio: {
    accessorKey: 'devices_offline_ratio',
    header: 'Offline / Total Devices',
    sortDescFirst: true,
    size: 45,
  },
  peak_usage: {
    accessorKey: 'peak_usage',
    header: 'Peak Usage',
    sortDescFirst: true,
    size: 80,
    Cell: (x) => (
      <div
        css={{
          gap: '4px',
          display: 'flex',
          alignItems: 'center',
          height: '100%',
        }}
      >
        <Typography variant="body2" noWrap={true}>
          {x.cell.getValue()}
        </Typography>
        {x.cell.getValue() >= 0 && x.row.original.total_devices && (
          <Typography variant="caption" sx={{ color: colors.grey[500] }}>
            (
            {
              // Capping the value if the percentage is higher than 100%, see b/473028358.
              (x.cell.getValue() / x.row.original.total_devices <= 1
                ? x.cell.getValue() / x.row.original.total_devices
                : 1
              ).toLocaleString('en-US', {
                style: 'percent',
              })
            }
            )
          </Typography>
        )}
      </div>
    ),

    Header: () => {
      return (
        <div
          css={{
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <Typography
            variant="subhead2"
            sx={{ overflowX: 'hidden', textOverflow: 'ellipsis' }}
          >
            Peak Usage
          </Typography>
          <InfoTooltip infoCss={{ marginLeft: '10px' }}>
            <Typography variant="body2">
              The maximum number of busy devices in the past 14 days
            </Typography>

            <Typography
              variant="caption"
              css={{ marginTop: 10, display: 'block' }}
            >
              The percentage is calculated as the ratio of 14 day peak active
              devices to the current total number of devices and is capped at
              100%.
            </Typography>
          </InfoTooltip>
        </div>
      );
    },
  },
  'static-omnilab_link': {
    accessorKey: 'static-omnilab_link',
    header: 'Explore in Arsenal',
    enableSorting: false,
    size: 15,
    muiTableBodyCellProps: {
      align: 'center',
    },
    Cell: (x) => {
      x.cell.getValue();
      // Double encodeURIComponent because omnilab is weird i guess
      const params = new URLSearchParams();
      if (x.row.original.lab_name)
        params.append(
          'host',
          'lab_location:include:' + encodeURIComponent(x.row.original.lab_name),
        );
      if (x.row.original.host_group)
        params.append(
          'host',
          'host_group:include:' + encodeURIComponent(x.row.original.host_group),
        );
      if (x.row.original.run_target)
        if (
          x.row
            .getValue<Row['host_group']>('host_group')
            ?.includes('crystalball')
        )
          // this team uses the run target label a bit differently so we are making an exeption for them
          params.append(
            'device',
            'product_board:include:' +
              encodeURIComponent(x.row.original.run_target),
          );
        else
          params.append(
            'device',
            'hardware:include:' + encodeURIComponent(x.row.original.run_target),
          );

      const to = `https://omnilab.corp.google.com/recovery?${params.toString()}`;

      return (
        <Link
          css={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
          }}
          to={to}
          target="_blank"
        >
          <OpenInNewIcon />
        </Link>
      );
    },
  },
  'static-explore_devices': {
    accessorKey: 'static-explore_devices',
    header: 'Explore devices',
    enableSorting: false,
    size: 15,
    muiTableBodyCellProps: {
      align: 'center',
    },
    Cell: (x) => {
      const filters = {
        lab_name: [x.row.original.lab_name],
        host_group: [x.row.original.host_group],
        run_target: [x.row.original.run_target],
      };

      const params = new URLSearchParams();
      params.set(ORDER_BY_PARAM_KEY, `state ${OrderByDirection.DESC}`);

      const to = `${generateDeviceListURL(ANDROID_PLATFORM)}${getFilterQueryString(filters, params)}`;

      return (
        <Link
          css={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
          }}
          to={to}
          target="_blank"
        >
          <OpenInNewIcon />
        </Link>
      );
    },
  },
} satisfies Partial<{
  [Key in keyof Row | `static-${string}`]: Key extends keyof Row
    ? MRT_ColumnDef<Row, Row[Key]> & {
        accessorKey: Key;
      }
    : MRT_ColumnDef<Row, undefined>;
}>;

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

  const onSelectedOptionsChange = (newSelectedOptions: SelectedOptions) => {
    setSearchParams(filtersUpdater(newSelectedOptions));

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

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
