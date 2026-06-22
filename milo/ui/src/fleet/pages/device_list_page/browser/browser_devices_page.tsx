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

import { Alert, Link } from '@mui/material';
import { useCallback, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { PlatformNotAvailable } from '@/fleet/components/platform_not_available';
import { getFeatureFlag } from '@/fleet/config/features';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AdminTasksAlert } from '../common/admin_tasks_alert';

import { BrowserDevicesTable } from './browser_devices_table';
import { BrowserSummaryHeader } from './browser_summary_header';
import { useBrowserColumns } from './use_browser_columns';
import { useBrowserFilters } from './use_browser_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];
const DEFAULT_PAGE_SIZE = 100;

export const BrowserDevicesPage = () => {
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
  } = useBrowserFilters(handleFilterChange);

  const { mrtColumnManager, warnings: columnWarnings } = useBrowserColumns(
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
      <BrowserSummaryHeader />
      <AdminTasksAlert />
      <Alert
        severity="info"
        sx={{
          marginTop: '24px',
        }}
      >
        Currently, this page only displays physical devices. virtualized
        hardware (VMs and GCE instances) will be onboarded by the end of Q2 or
        early Q3 (tracking bug:{' '}
        <Link href="http://b/503171517" target="_blank" rel="noreferrer">
          b/503171517
        </Link>
        ).
      </Alert>
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
          searchPlaceholder='Add a filter (e.g. "os:Linux" or "sw.pool:default")'
        />
      </div>
      <div
        css={{
          marginTop: 24,
        }}
      >
        <BrowserDevicesTable mrtColumnManager={mrtColumnManager} />
      </div>
    </div>
  );
};

export function Component() {
  const isSupported = getFeatureFlag('BrowserListDevices');

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <FleetHelmet pageTitle="Device List" />
      <RecoverableErrorBoundary key="fleet-device-list-page">
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
