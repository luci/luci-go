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
// limitations under the License.
import { useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { DeviceTable } from '@/fleet/components/device_table';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { MainMetrics } from '@/fleet/components/main_metrics';
import { MultiSelectFilter } from '@/fleet/components/multi_select_filter';
import {
  filtersUpdater,
  getFilters,
  getFilterValue,
  stringifyFilters,
} from '@/fleet/components/multi_select_filter/search_param_utils/search_param_utils';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useDevices } from '@/fleet/hooks/use_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { SelectedOptions } from '@/fleet/types';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountDevicesRequest,
  ListDevicesRequest,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { dimensionsToFilterOptions, filterOptionsPlaceholder } from './helpers';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

export const DeviceListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const [selectedOptions, setSelectedOptions] = useState<SelectedOptions>(
    getFilters(searchParams),
  );

  const onSelectedOptionsChange = (newSelectedOptions: SelectedOptions) => {
    setSelectedOptions(newSelectedOptions);
    setSearchParams(filtersUpdater(newSelectedOptions));

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const client = useFleetConsoleClient();
  const dimensionsQuery = useQuery(client.GetDeviceDimensions.query({}));
  const countQuery = useQuery(
    client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: stringifyFilters(selectedOptions),
      }),
    ),
  );
  const request = ListDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: getFilterValue(searchParams),
  });

  const devicesQuery = useDevices(request);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};
  const columns = dimensionsQuery.data
    ? _.uniq(
        Object.keys(dimensionsQuery.data.baseDimensions).concat(
          Object.keys(dimensionsQuery.data.labels),
        ),
      )
    : [];

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <MainMetrics countQuery={countQuery} />
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
        <MultiSelectFilter
          filterOptions={
            dimensionsQuery.data
              ? dimensionsToFilterOptions(dimensionsQuery.data)
              : filterOptionsPlaceholder(selectedOptions)
          }
          selectedOptions={selectedOptions}
          onSelectedOptionsChange={onSelectedOptionsChange}
          isLoading={dimensionsQuery.isLoading}
        />
      </div>
      <div
        css={{
          borderRadius: 4,
          marginTop: 24,
        }}
      >
        <DeviceTable
          devices={devices}
          columnIds={columns}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
          isError={devicesQuery.isError || dimensionsQuery.isError}
          error={devicesQuery.error || dimensionsQuery.error}
          isLoading={devicesQuery.isLoading}
          isLoadingColumns={dimensionsQuery.isLoading}
          totalRowCount={countQuery?.data?.total}
        />
      </div>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-list-page"
      >
        <LoggedInBoundary>
          <DeviceListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
