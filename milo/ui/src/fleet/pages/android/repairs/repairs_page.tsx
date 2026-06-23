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
import { Alert, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  PagerContext,
  usePagerContext,
} from '@/common/components/params_pager';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import {
  ANDROID_PLATFORM,
  generateDeviceListURL,
} from '@/fleet/constants/paths';
import { ORDER_BY_PARAM_KEY } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Platform,
  RepairMetric_Priority,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getPriorityIcon } from './repairs_columns.utils';
import { RepairsTable } from './repairs_table';
import { useRepairsColumns } from './use_repairs_columns';
import { useRepairsFilters } from './use_repairs_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

export const RepairListPage = () => {
  const [, setSearchParams] = useSyncedSearchParams();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const handleFilterChange = useCallback(() => {
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  }, [pagerCtx, setSearchParams]);

  const {
    filterValues,
    aip160,
    warnings: filterWarnings,
    isLoading: isLoadingFilters,
  } = useRepairsFilters(handleFilterChange);

  const { mrtColumnManager, warnings: columnWarnings } = useRepairsColumns(
    filterValues,
    isLoadingFilters || filterValues === undefined,
  );

  const combinedWarnings = useMemo(
    () => [...(filterWarnings || []), ...(columnWarnings || [])],
    [filterWarnings, columnWarnings],
  );

  return (
    <div
      css={{
        margin: '24px',
        paddingBottom: '40px',
      }}
    >
      <WarningNotifications warnings={combinedWarnings} />
      <Metrics
        filters={aip160()}
        pagerContext={pagerCtx}
        filterValues={filterValues}
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
          filterCategoryDatas={Object.values(filterValues || {})}
          isLoading={isLoadingFilters || filterValues === undefined}
          searchPlaceholder='Add a filter (e.g. "state:ready" or "priority:high")'
        />
      </div>
      <div
        css={{
          borderRadius: 4,
          marginTop: 24,
        }}
      >
        <RepairsTable mrtColumnManager={mrtColumnManager} />
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
  const [searchParams] = useSyncedSearchParams();
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
      <RecoverableErrorBoundary key="fleet-repairs">
        <LoggedInBoundary>
          <RepairListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
