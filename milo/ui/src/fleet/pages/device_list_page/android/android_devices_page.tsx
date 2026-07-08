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

import { useCallback, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { AndroidSummaryHeader } from '@/fleet/pages/device_list_page/android/android_summary_header';
import { AdminTasksAlert } from '@/fleet/pages/device_list_page/common/admin_tasks_alert';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { AndroidDevicesTable } from './android_devices_table';
import { useAndroidColumns } from './use_android_columns';
import { useAndroidFilters } from './use_android_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

export const AndroidDevicesPage = () => {
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
    isLoading,
    warnings: filterWarnings,
    setFiltersBatch,
    aip160,
  } = useAndroidFilters(handleFilterChange);

  const {
    mrtColumnManager,
    warnings: columnWarnings,
    availableColumns,
  } = useAndroidColumns(
    filterValues,
    isLoading || filterValues === undefined,
    false,
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
      <AndroidSummaryHeader
        aip160={aip160()}
        setFiltersBatch={setFiltersBatch}
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
          filterCategoryDatas={Object.values(filterValues || {})}
          isLoading={isLoading || filterValues === undefined}
          searchPlaceholder='Add a filter (e.g. "state:idle", "pool:default", or "device_id:123")'
        />
      </div>
      <div
        css={{
          marginTop: 24,
        }}
      >
        <AndroidDevicesTable
          mrtColumnManager={mrtColumnManager}
          availableColumns={availableColumns}
        />
      </div>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary key="fleet-device-list-page">
        <LoggedInBoundary>
          <AndroidDevicesPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
