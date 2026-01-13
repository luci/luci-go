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

import { Chip } from '@mui/material';
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
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import {
  filtersUpdater,
  getFilters,
} from '@/fleet/components/filter_dropdown/search_param_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { CHROMIUM_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { getFeatureFlag } from '@/fleet/config/features';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useBrowserDevices } from '@/fleet/hooks/use_browser_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { SelectedOptions } from '@/fleet/types';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ListBrowserDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AutorepairJobsAlert } from '../common/autorepair_jobs_alert';
import { filterOptionsPlaceholder } from '../common/helpers';

import { BrowserSummaryHeader } from './browser_summary_header';
import { getColumn } from './columns';
import { dimensionsToFilterOptions } from './dimensions_to_filter_options';
import { useBrowserDeviceDimensions } from './use_browser_device_dimensions';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;
const EXTRA_COLUMN_IDS = ['id'];

export const BrowserDevicesPage = () => {
  const { trackEvent } = useGoogleAnalytics();
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
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });

    setSearchParams(filtersUpdater(newSelectedOptions));

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const stringifiedSelectedOptions = selectedOptions.error
    ? ''
    : stringifyFilters(selectedOptions.filters);

  const dimensionsQuery = useBrowserDeviceDimensions();

  // TODO: b/390013758 - Use the real CountDevices RPC once it's ready for CHROMIUM platform.
  const countQuery = {
    data: { total: 0 },
    isPending: false,
    isError: false,
    error: null,
  };

  const request = ListBrowserDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: stringifiedSelectedOptions,
  });

  const devicesQuery = useBrowserDevices(request);

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};
  const columnIds = useMemo(() => {
    const ids: string[] = [];
    if (dimensionsQuery.isSuccess) {
      ids.push(
        ...Object.keys(dimensionsQuery.data.baseDimensions)
          .concat(...Object.keys(dimensionsQuery.data.swarmingLabels))
          .concat(...Object.keys(dimensionsQuery.data.ufsLabels)),
      );
    }

    if (devicesQuery.data) {
      return _.uniq([
        ...devicesQuery.data.devices.flatMap((d) => [
          ...Object.keys(d.swarmingLabels ?? {}).map(
            (l) => `${BROWSER_SWARMING_SOURCE}."${l}"`,
          ),
          ...Object.keys(d.ufsLabels ?? {}).map(
            (l) => `${BROWSER_UFS_SOURCE}."${l}"`,
          ),
        ]),
      ]);
    }

    return _.uniq([...ids, ...EXTRA_COLUMN_IDS]);
  }, [
    devicesQuery.data,
    dimensionsQuery.data?.baseDimensions,
    dimensionsQuery.data?.swarmingLabels,
    dimensionsQuery.data?.ufsLabels,
    dimensionsQuery.isSuccess,
  ]);

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (dimensionsQuery.isPending) return;

    const missingParamsColoumns = getWrongColumnsFromParams(
      searchParams,
      columnIds,
      CHROMIUM_DEFAULT_COLUMNS,
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
    columnIds,
    dimensionsQuery.isPending,
    searchParams,
    setSearchParams,
  ]);

  useEffect(() => {
    if (selectedOptions.error) return;
    if (!dimensionsQuery.isSuccess) return;

    const missingParamsFilters = Object.keys(selectedOptions.filters).filter(
      (filterKey) => !columnIds.includes(filterKey),
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
    addWarning,
    dimensionsQuery,
    selectedOptions,
    setSearchParams,
    columnIds,
  ]);

  useEffect(() => {
    if (!selectedOptions.error) return;
    addWarning('Invalid filters');
    setSearchParams(filtersUpdater({}));
  }, [addWarning, selectedOptions.error, setSearchParams]);

  const columnsRecord = useMemo(
    () => Object.fromEntries(columnIds.map((id) => [id, getColumn(id)])),
    [columnIds],
  );

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <BrowserSummaryHeader selectedOptions={selectedOptions.filters || {}} />
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
              dimensionsQuery.isSuccess
                ? dimensionsToFilterOptions(dimensionsQuery.data, columnsRecord)
                : filterOptionsPlaceholder(
                    selectedOptions.filters,
                    columnsRecord,
                  )
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
          defaultColumnIds={CHROMIUM_DEFAULT_COLUMNS}
          localStorageKey={BROWSER_DEVICES_LOCAL_STORAGE_KEY}
          rows={devices}
          availableColumns={Object.values(columnsRecord)}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
          isError={devicesQuery.isError || dimensionsQuery.isError}
          error={devicesQuery.error || dimensionsQuery.error}
          isLoading={devicesQuery.isPending || devicesQuery.isPlaceholderData}
          isLoadingColumns={dimensionsQuery.isPending}
          totalRowCount={countQuery?.data?.total}
        />
      </div>
    </div>
  );
};

export function Component() {
  const isSupported = getFeatureFlag('BrowserListDevices');

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-list-page"
      >
        <LoggedInBoundary>
          {isSupported ? (
            <BrowserDevicesPage />
          ) : (
            <PlatformNotAvailable
              availablePlatforms={[Platform.CHROMEOS, Platform.ANDROID]}
            />
          )}
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
