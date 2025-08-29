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
import { Alert, Chip, Divider, Typography } from '@mui/material';
import { GridColDef, GridColumnHeaderTitle } from '@mui/x-data-grid';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useEffect, useMemo } from 'react';
import { Link } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { FilterBarOld } from '@/fleet/components/filter_dropdown/filter_bar_old';
import {
  filtersUpdater,
  getFilters,
  stringifyFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils/search_param_utils';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { colors } from '@/fleet/theme/colors';
import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Platform,
  RepairMetric_Priority,
  repairMetric_PriorityToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

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

const COLUMNS: Record<string, GridColDef> = {
  priority: {
    field: 'priority',
    flex: 1,
    renderHeader: () => {
      return (
        <>
          <GridColumnHeaderTitle label="Priority" columnWidth={Infinity} />
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
        </>
      );
    },
    renderCell: (x) => (
      <div
        css={{
          gap: '4px',
          display: 'flex',
          alignItems: 'center',
          height: '100%',
        }}
      >
        {getPriorityIcon(x.value as RepairMetric_Priority)}
        <Typography variant="body2" noWrap={true}>
          {repairMetric_PriorityToJSON(x.value)}
        </Typography>
      </div>
    ),
  },
  labName: {
    field: 'lab_name',
    headerName: 'Lab Name',
    flex: 1,
  },
  hostGroup: {
    field: 'host_group',
    headerName: 'Host Group',
    flex: 2,
  },
  runTarget: {
    field: 'run_target',
    headerName: 'Run Target',
    flex: 1,
  },
  minimumRepairs: {
    field: 'minimum_repairs',
    headerName: 'Minimum Repairs',
    flex: 1,
    renderHeader: () => {
      return (
        <>
          <GridColumnHeaderTitle
            label="Minimum Repairs"
            columnWidth={Infinity}
          />
          <InfoTooltip infoCss={{ marginLeft: '10px' }}>
            The minimum number of repairs needed to meet SLOs
          </InfoTooltip>
        </>
      );
    },
  },
  devicesOfflinePercentage: {
    field: 'devices_offline_percentage',
    headerName: 'Devices Offline %',
    flex: 1,
    renderHeader: () => {
      return (
        <Typography
          variant="subhead2"
          css={{
            fontWeight: 500,
            textWrap: 'wrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          }}
        >
          Devices Offline&nbsp;%
        </Typography>
      );
    },
  },
  devicesOfflineRatio: {
    field: 'devices_offline_ratio',
    headerName: 'Offline / Total Devices',
    flex: 1,
  },
  peakUsage: {
    field: 'peakUsage',
    headerName: 'Peak usage',
    flex: 1,
  },
  omnilab_link: {
    field: 'omnilab_link',
    headerName: 'Explore in Arsenal',
    flex: 1,
    renderCell: (x) => {
      // Double encodeURIComponent because omnilab is weird i guess
      const params = new URLSearchParams();
      if (x.row.lab_name)
        params.append(
          'host',
          'lab_location:include:' + encodeURIComponent(x.row.lab_name),
        );
      if (x.row.host_group)
        params.append(
          'host',
          'host_group:include:' + encodeURIComponent(x.row.host_group),
        );
      if (x.row.run_target)
        params.append(
          'device',
          'hardware:include:' + encodeURIComponent(x.row.run_target),
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
};

export const RepairListPage = ({ platform }: { platform: Platform }) => {
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
      platform: platform,
      filter: stringifiedSelectedOptions,
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
      orderBy: orderByParam,
    }),
    placeholderData: keepPreviousData,
    enabled: platform !== undefined,
  });

  const repairMetricsFilterValues = useQuery({
    ...client.GetRepairMetricsDimensions.query({
      platform: platform,
    }),
    enabled: platform !== undefined,
  });

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (selectedOptions.error) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) => !COLUMNS[_.camelCase(filterKey)],
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

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <Metrics filters={stringifiedSelectedOptions} platform={platform} />
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
        {/* TODO add error message if repairMetricsList fails */}
        <StyledGrid
          columns={Object.values(COLUMNS)}
          rows={repairMetricsList.data?.repairMetrics.map((rm) => ({
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
            peakUsage: rm.peakUsage,
          }))}
          slots={{
            pagination: Pagination,
          }}
          slotProps={{
            pagination: {
              pagerCtx: pagerCtx,
              nextPageToken: repairMetricsList.data?.nextPageToken,
              totalRowCount: repairMetricsList.data?.totalSize,
            },
          }}
          rowSelection={false}
          sortingMode="server"
          rowCount={repairMetricsList.data?.totalSize}
          paginationMode="server"
          paginationModel={{
            page: getCurrentPageIndex(pagerCtx),
            pageSize: getPageSize(pagerCtx, searchParams),
          }}
          onSortModelChange={(newModel) => {
            if (newModel.length !== 1) {
              updateOrderByParam('');
              setSearchParams(emptyPageTokenUpdater(pagerCtx));
              return;
            }

            const by = newModel[0].field;
            const order = newModel[0].sort === 'asc' ? '' : 'desc';
            updateOrderByParam(`${by} ${order}`);
            setSearchParams(emptyPageTokenUpdater(pagerCtx));
          }}
          loading={repairMetricsList.isPending}
          disableColumnFilter={true}
          disableColumnSelector
          disableColumnMenu
        />
      </div>
    </div>
  );
};

function Metrics({
  filters,
  platform,
}: {
  filters: string;
  platform: Platform;
}) {
  const client = useFleetConsoleClient();
  const countQuery = useQuery({
    ...client.CountRepairMetrics.query({
      platform: platform,
      filter: filters,
    }),
    enabled: platform !== undefined,
  });

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
            />
            <SingleMetric
              name="Distinct Hosts Offline"
              value={countQuery.data?.offlineHosts}
              total={countQuery.data?.totalHosts}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
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
            />
            <SingleMetric
              name="Distinct Devices Offline"
              value={countQuery.data?.offlineDevices}
              total={countQuery.data?.totalDevices}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
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
            />
            <SingleMetric
              name="Watch"
              value={countQuery.data?.watchRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.WATCH)}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Nice"
              value={countQuery.data?.niceRepairGroup}
              total={countQuery.data?.totalRepairGroup}
              Icon={getPriorityIcon(RepairMetric_Priority.NICE)}
              loading={countQuery.isPending}
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
