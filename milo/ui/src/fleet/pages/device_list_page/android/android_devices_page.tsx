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

import _ from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { DeviceTable } from '@/fleet/components/device_table';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilters } from '@/fleet/components/filters/use_filters';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getWrongColumnsFromParams } from '@/fleet/utils/get_wrong_columns_from_params';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import {
  TrackLeafRoutePageView,
  useGoogleAnalytics,
} from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ListDevicesRequest,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { AdminTasksAlert } from '../common/admin_tasks_alert';
import { dimensionsToFilterOptions } from '../common/helpers';
import { useDeviceDimensions } from '../common/use_device_dimensions';

import { ANDROID_COLUMN_OVERRIDES, getAndroidColumns } from './android_columns';
import { ANDROID_EXTRA_FILTERS } from './android_filters';
import { AndroidSummaryHeader } from './android_summary_header';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

const platform = Platform.ANDROID;

const EXTRA_COLUMN_IDS = [
  'label-id',
  'label-run_target',
  'label-hostname',
  'fc_offline_since',
  'realm',
] satisfies (keyof typeof ANDROID_COLUMN_OVERRIDES)[];

export const AndroidDevicesPage = () => {
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const dimensionsQuery = useDeviceDimensions({ platform });

  // TODO: b/419764393, b/420287987 - In local storage REACT_QUERY_OFFLINE_CACHE can contain empty data object which causes app to crash.
  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.labels;

  const filterOptions = isDimensionsQueryProperlyLoaded
    ? {
        ...ANDROID_EXTRA_FILTERS,
        ...dimensionsToFilterOptions(
          dimensionsQuery.data!,
          ANDROID_COLUMN_OVERRIDES,
        ),
      }
    : {
        ...ANDROID_EXTRA_FILTERS,
        '"lab_name"': new StringListFilterCategoryBuilder()
          .setLabel('lab_name')
          .setOptions([
            {
              label: 'atc',
              key: '"atc"',
            },
          ]),
      };
  const filterCategoryDatas = useFilters(filterOptions);

  const onApplyFilter = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  }, [pagerCtx, setSearchParams, trackEvent]);

  const request = ListDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: orderByParam,
    filter: filterCategoryDatas.parseError ? '' : filterCategoryDatas.aip160,
    platform: platform,
  });

  const devicesQuery = useAndroidDevices(request);

  const {
    devices = [],
    nextPageToken = '',
    totalSize = 0,
  } = devicesQuery.data || {};
  const columnIds = useMemo(() => {
    const ids: string[] = [];
    if (isDimensionsQueryProperlyLoaded) {
      ids.push(
        ...Object.keys(dimensionsQuery.data.baseDimensions).concat(
          Object.keys(dimensionsQuery.data.labels),
        ),
      );
    }

    if (devicesQuery.data) {
      ids.push(
        ...devicesQuery.data.devices.flatMap((d) =>
          Object.keys(d.omnilabSpec?.labels ?? {}),
        ),
      );
    }

    return _.uniq([...ids, ...EXTRA_COLUMN_IDS]);
  }, [
    isDimensionsQueryProperlyLoaded,
    dimensionsQuery.data,
    devicesQuery.data,
  ]);

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (dimensionsQuery.isPending || devicesQuery.isPending) return;

    const missingParamsColumns = getWrongColumnsFromParams(
      searchParams,
      columnIds,
      ANDROID_DEFAULT_COLUMNS,
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
    for (const fc of Object.values(filterCategoryDatas.filterValues || {})) {
      fc.clear();
    }
  }, [
    addWarning,
    filterCategoryDatas,
    isDimensionsQueryProperlyLoaded,
    warnings,
  ]);

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <WarningNotifications warnings={warnings} />
      <AndroidSummaryHeader
        aip160={
          filterCategoryDatas.parseError ? '' : filterCategoryDatas.aip160
        }
        filters={filterCategoryDatas.filterValues}
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
        />
      </div>
      <div
        css={{
          borderRadius: 4,
          marginTop: 24,
        }}
      >
        <DeviceTable
          defaultColumnIds={ANDROID_DEFAULT_COLUMNS}
          localStorageKey={ANDROID_DEVICES_LOCAL_STORAGE_KEY}
          rows={devices}
          availableColumns={getAndroidColumns(columnIds)}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
          isError={devicesQuery.isError || dimensionsQuery.isError}
          error={devicesQuery.error || dimensionsQuery.error}
          isLoading={devicesQuery.isPending || devicesQuery.isPlaceholderData}
          isLoadingColumns={dimensionsQuery.isPending || devicesQuery.isPending}
          totalRowCount={totalSize}
          getRowId={(row) =>
            row.id + (row.omnilabSpec?.labels?.fc_machine_type?.values ?? '')
          }
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
          <AndroidDevicesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
