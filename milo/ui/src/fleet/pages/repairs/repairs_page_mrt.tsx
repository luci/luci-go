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
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Chip, Typography } from '@mui/material';
import { colors } from '@mui/material';
import {
  keepPreviousData,
  useQuery,
  type UseQueryResult,
} from '@tanstack/react-query';
import _ from 'lodash';
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Cell,
  type MRT_Row,
  MRT_PaginationState,
} from 'material-react-table';
import { useEffect, useMemo } from 'react';
import { Link } from 'react-router';

// Augment the MRT_ColumnDef type to include a custom 'sortKey' property.
declare module 'material-react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-explicit-any
  interface MRT_ColumnDef<TData extends Record<string, any>> {
    sortKey?: string;
  }
}

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { FilterBarOld } from '@/fleet/components/filter_dropdown/filter_bar_old';
import {
  filtersUpdater,
  getFilters,
  stringifyFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils/search_param_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import {
  ORDER_BY_PARAM_KEY,
  OrderByDirection,
  useOrderByParam,
} from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { useFCDataTable } from '@/fleet/hooks/use_fc_data_table';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { parseOrderByParam } from '@/fleet/utils/search_param';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountRepairMetricsResponse,
  Platform,
  RepairMetric,
  RepairMetric_Priority,
  repairMetric_PriorityToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

export const RepairListPage = ({ platform }: { platform: Platform }) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const client = useFleetConsoleClient();

  const columns = useMemo<MRT_ColumnDef<RepairMetric>[]>(
    () => [
      {
        id: 'priority',
        accessorKey: 'priority',
        sortKey: 'priority',
        header: 'Priority',
        Cell: ({ cell }: { cell: MRT_Cell<RepairMetric, unknown> }) => {
          const label = repairMetric_PriorityToJSON(cell.getValue<number>());

          let Icon: React.ReactNode = null;
          switch (cell.getValue<RepairMetric_Priority>()) {
            case RepairMetric_Priority.NICE:
              Icon = <DoneIcon sx={{ color: colors.green[400] }} />;
              break;
            case RepairMetric_Priority.MISSING_DATA:
            case RepairMetric_Priority.DEVICES_REMOVED:
            case RepairMetric_Priority.WATCH:
              Icon = <WarningIcon sx={{ color: colors.yellow[900] }} />;
              break;
            case RepairMetric_Priority.BREACHED:
              Icon = <ErrorIcon sx={{ color: colors.red[500] }} />;
              break;
          }

          return (
            <div
              css={{
                gap: '4px',
                display: 'flex',
                alignItems: 'center',
                height: '100%',
              }}
            >
              {Icon}
              <Typography>{label}</Typography>
            </div>
          );
        },
      },
      {
        id: 'labName',
        accessorKey: 'labName',
        sortKey: 'lab_name',
        header: 'Lab Name',
      },
      {
        id: 'hostGroup',
        accessorKey: 'hostGroup',
        sortKey: 'host_group',
        header: 'Host Group',
      },
      {
        id: 'runTarget',
        accessorKey: 'runTarget',
        sortKey: 'run_target',
        header: 'Run Target',
      },
      {
        id: 'minimumRepairs',
        accessorKey: 'minimumRepairs',
        sortKey: 'minimum_repairs',
        header: 'Minimum Repairs',
      },
      {
        id: 'devicesOfflineRatio',
        header: 'Devices Offline / Total Devices',
        accessorFn: (row: RepairMetric) =>
          `${row.devicesOffline} / ${row.totalDevices}`,
        sortKey: 'devices_offline',
      },
      {
        id: 'omnilabLink',
        header: 'Explore in arsenal',
        enableSorting: false,
        muiTableBodyCellProps: {
          align: 'center',
        },
        Cell: ({ row }: { row: MRT_Row<RepairMetric> }) => {
          // Double encodeURIComponent because omnilab is weird i guess
          const to = `https://omnilab.corp.google.com/recovery?host=host_group:include:${encodeURIComponent(encodeURIComponent(row.original.hostGroup))}&device=Hardware:include:${encodeURIComponent(encodeURIComponent(row.original.runTarget))}`;

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
    ],
    [],
  );

  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const selectedOptions = useMemo(
    () => getFilters(searchParams),
    [searchParams],
  );

  const onSelectedOptionsChange = (newSelectedOptions: SelectedOptions) => {
    setSearchParams(
      (prev) => {
        const next = filtersUpdater(newSelectedOptions)(prev);
        return emptyPageTokenUpdater(pagerCtx)(next);
      },
      { replace: true },
    );
  };

  const stringifiedSelectedOptions = selectedOptions.error
    ? ''
    : stringifyFilters(selectedOptions.filters);

  const orderBy = parseOrderByParam(searchParams.get(ORDER_BY_PARAM_KEY) ?? '');

  let sorting =
    orderBy !== null
      ? [
          {
            id: columns.filter((c) =>
              [c.id, c.sortKey].includes(orderBy.field),
            )[0].id as string,
            desc: orderBy.direction === OrderByDirection.DESC,
          },
        ]
      : [];

  const pagination = useMemo<MRT_PaginationState>(
    () => ({
      pageIndex: getCurrentPageIndex(pagerCtx),
      pageSize: getPageSize(pagerCtx, searchParams),
    }),
    [pagerCtx, searchParams],
  );

  const repairMetricsQuery = useQuery({
    ...client.ListRepairMetrics.query({
      platform: platform,
      filter: stringifiedSelectedOptions,
      pageSize: pagination.pageSize,
      pageToken: getPageToken(pagerCtx, searchParams),
      orderBy: orderByParam,
    }),
    placeholderData: keepPreviousData,
    enabled: platform !== undefined,
  });

  const {
    repairMetrics = [],
    nextPageToken = '',
    totalSize = 0,
  } = repairMetricsQuery.data || {};

  const rows = useMemo(
    () => repairMetrics.slice(0, pagination.pageSize),
    [repairMetrics, pagination],
  );

  const repairMetricsFilterValues = useQuery({
    ...client.GetRepairMetricsDimensions.query({
      platform: platform,
    }),
    enabled: platform !== undefined,
  });

  const countQuery = useQuery({
    ...client.CountRepairMetrics.query({
      platform: platform,
      filter: stringifiedSelectedOptions,
    }),
    enabled: platform !== undefined,
  });

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (selectedOptions.error) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) =>
        !columns.some((c) => c.accessorKey === _.camelCase(filterKey)),
    );
    if (missingParamsFilters.length === 0) return;
    addWarning(
      'The following filters are not available: ' +
        missingParamsFilters?.join(', '),
    );
    for (const key of missingParamsFilters) {
      delete selectedOptions.filters[key];
    }
    setSearchParams((prev) => filtersUpdater(selectedOptions.filters)(prev), {
      replace: true,
    });
  }, [addWarning, selectedOptions, setSearchParams, columns]);

  useEffect(() => {
    if (!selectedOptions.error) return;
    addWarning('Invalid filters');
    setSearchParams((prev) => filtersUpdater({})(prev), {
      replace: true,
    });
  }, [addWarning, selectedOptions.error, setSearchParams]);

  const table = useFCDataTable({
    columns: columns,
    data: rows,
    getRowId: (row) => row.labName + row.hostGroup + row.runTarget,
    state: {
      sorting,
      isLoading: repairMetricsQuery.isLoading,
      showAlertBanner: repairMetricsQuery.isError,
      pagination: pagination,
    },
    muiPaginationProps: {
      rowsPerPageOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    },
    muiToolbarAlertBannerProps: repairMetricsQuery.isError
      ? {
          color: 'error',
          children: getErrorMessage(
            repairMetricsQuery.error,
            'get repair metrics',
          ),
        }
      : undefined,

    // Sorting
    manualSorting: true,
    onSortingChange: (updater) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;
      sorting = newSorting;

      if (newSorting.length !== 1) {
        updateOrderByParam('');
        setSearchParams((prev) => emptyPageTokenUpdater(pagerCtx)(prev), {
          replace: true,
        });
        return;
      }

      const by =
        newSorting[0].id === 'devicesOfflineRatio'
          ? 'devices_offline'
          : (table.getColumn(newSorting[0].id).columnDef.sortKey ??
            newSorting[0].id);

      const order = newSorting[0].desc ? 'desc' : '';
      updateOrderByParam(`${by} ${order}`);
      setSearchParams((prev) => emptyPageTokenUpdater(pagerCtx)(prev), {
        replace: true,
      });
    },

    // Pagination
    rowCount: totalSize,
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

      setSearchParams((prev) => {
        let next = pageSizeUpdater(pagerCtx, newPagination.pageSize)(prev);
        if (isPrevPage) {
          next = prevPageTokenUpdater(pagerCtx)(next);
        } else if (isNextPage) {
          next = nextPageTokenUpdater(pagerCtx, nextPageToken)(next);
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
      <Metrics countQuery={countQuery} />
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
          <FilterBarOld
            filterOptions={Object.entries(
              repairMetricsFilterValues.data?.dimensions ?? {},
            ).map(
              ([key, val]): OptionCategory => ({
                label: _.startCase(key),
                value: key,
                options: val.values.map((v) => ({
                  label: v,
                  value: v,
                })),
              }),
            )}
            selectedOptions={selectedOptions.filters}
            onSelectedOptionsChange={onSelectedOptionsChange}
            isLoading={repairMetricsFilterValues.isPending}
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
  countQuery,
}: {
  countQuery: UseQueryResult<CountRepairMetricsResponse, Error>;
}) {
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
      <div css={{ marginTop: 24 }}>
        {countQuery.isError ? (
          <Alert severity="error">
            {getErrorMessage(countQuery.error, 'get the main metrics')}
          </Alert>
        ) : (
          <div
            css={{
              display: 'flex',
              maxWidth: 800,
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
                  justifyContent: 'space-around',
                  marginTop: 5,
                  flexWrap: 'wrap',
                }}
              >
                <SingleMetric
                  name="Total Hosts"
                  value={countQuery.data?.totalHosts}
                  loading={countQuery.isPending}
                />
                <SingleMetric
                  name="Distinct Hosts Offline"
                  value={countQuery.data?.offlineHosts}
                  total={countQuery.data?.totalHosts}
                  Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
                  loading={countQuery.isPending}
                />
              </div>
            </div>
            <div
              css={{
                display: 'flex',
                flexDirection: 'column',
                flexGrow: 1,
              }}
            >
              <Typography variant="subhead1">Devices</Typography>
              <div
                css={{
                  display: 'flex',
                  marginTop: 5,
                  marginLeft: 8,
                  flexWrap: 'wrap',
                }}
              >
                <SingleMetric
                  name="Total"
                  value={countQuery.data?.totalDevices}
                  loading={countQuery.isPending}
                />
                <SingleMetric
                  name="Distinct Devices Offline"
                  value={countQuery.data?.offlineDevices}
                  total={countQuery.data?.totalDevices}
                  Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
                  loading={countQuery.isPending}
                />
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export function Component() {
  const { platform } = usePlatform();

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-repairs">
      <FleetHelmet pageTitle="Repairs" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-repairs"
      >
        <LoggedInBoundary>
          {platform !== Platform.ANDROID ? (
            <PlatformNotAvailable availablePlatforms={[Platform.ANDROID]} />
          ) : (
            <RepairListPage platform={platform} />
          )}
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
