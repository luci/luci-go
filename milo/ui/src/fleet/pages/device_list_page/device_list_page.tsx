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

import { Chip } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useEffect, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { DeviceTable } from '@/fleet/components/device_table';
import { DeviceListFilterBar } from '@/fleet/components/device_table/device_list_filter_ft_selector';
import { useCurrentTasks } from '@/fleet/components/device_table/use_current_tasks';
import {
  filtersUpdater,
  getFilters,
  stringifyFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils/search_param_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { DEFAULT_DEVICE_COLUMNS } from '@/fleet/config/device_config';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { useDevices } from '@/fleet/hooks/use_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { MainMetricsContainer } from '@/fleet/pages/device_list_page/main_metrics';
import { SelectedOptions } from '@/fleet/types';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountDevicesRequest,
  ListDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AutorepairJobsAlert } from './autorepair_jobs_alert';
import { dimensionsToFilterOptions, filterOptionsPlaceholder } from './helpers';
import { useDeviceDimensions } from './use_device_dimensions';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

export const DeviceListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
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
  const dimensionsQuery = useDeviceDimensions();

  // TODO: b/419764393, b/420287987 - In local storage REACT_QUERY_OFFLINE_CACHE can contain empty data object which causes app to crash.
  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.labels;

  const countQuery = useQuery({
    ...client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: stringifiedSelectedOptions,
      }),
    ),
  });

  const request = ListDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: stringifiedSelectedOptions,
  });

  const devicesQuery = useDevices(request);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};
  const currentTasks = useCurrentTasks(devices);
  const columns = useMemo(
    () =>
      isDimensionsQueryProperlyLoaded
        ? _.uniq(
            Object.keys(dimensionsQuery.data.baseDimensions)
              .concat(Object.keys(dimensionsQuery.data.labels))
              .concat('current_task'),
          )
        : [],
    [isDimensionsQueryProperlyLoaded, dimensionsQuery.data],
  );

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (dimensionsQuery.isPending) return;

    const missingParamsColoumns = getWrongColumnsFromParams(
      searchParams,
      columns,
      DEFAULT_DEVICE_COLUMNS,
    );
    if (missingParamsColoumns.length === 0) return;
    addWarning(
      'The following columns are not available: ' +
        missingParamsColoumns?.join(', '),
    );
    for (const col of missingParamsColoumns) {
      searchParams.delete(COLUMNS_PARAM_KEY, col);
    }
    if (searchParams.getAll(COLUMNS_PARAM_KEY).length <= 1)
      searchParams.delete(COLUMNS_PARAM_KEY);

    setSearchParams(searchParams);
  }, [
    addWarning,
    columns,
    dimensionsQuery.isPending,
    searchParams,
    setSearchParams,
  ]);

  useEffect(() => {
    if (selectedOptions.error) return;
    if (!dimensionsQuery.isSuccess) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) =>
        isDimensionsQueryProperlyLoaded &&
        !dimensionsQuery.data.labels[filterKey.replace('labels.', '')] &&
        !dimensionsQuery.data.baseDimensions[filterKey],
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
  }, [
    isDimensionsQueryProperlyLoaded,
    addWarning,
    dimensionsQuery,
    selectedOptions,
    setSearchParams,
  ]);

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
      <MainMetricsContainer selectedOptions={selectedOptions.filters || {}} />
      <AutorepairJobsAlert />
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
              isDimensionsQueryProperlyLoaded
                ? dimensionsToFilterOptions(dimensionsQuery.data)
                : filterOptionsPlaceholder(selectedOptions.filters)
            }
            selectedOptions={selectedOptions.filters}
            onSelectedOptionsChange={onSelectedOptionsChange}
            isLoading={dimensionsQuery.isPending}
          />
        )}
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
          isError={
            devicesQuery.isError ||
            dimensionsQuery.isError ||
            currentTasks.isError
          }
          error={
            devicesQuery.error || dimensionsQuery.error || currentTasks.error
          }
          isLoading={devicesQuery.isPending}
          isLoadingColumns={dimensionsQuery.isPending}
          totalRowCount={countQuery?.data?.total}
          currentTaskMap={currentTasks.map}
        />
      </div>
    </div>
  );
};

export function Component() {
  const { platform } = usePlatform();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-list-page"
      >
        <LoggedInBoundary>
          {platform !== Platform.CHROMEOS ? (
            <PlatformNotAvailable availablePlatforms={[Platform.CHROMEOS]} />
          ) : (
            <DeviceListPage />
          )}
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
