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

import { Box, Typography } from '@mui/material';
import { useCallback, useMemo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { useChromeOSFilters } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_filters';
import { colors } from '@/fleet/theme/colors';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ChromeOSRepairTable } from './chromeos_repair_table';
import { useRepairQueueColumns } from './use_repair_queue_columns';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 100;

export const ChromeOSRepairDashboard = () => {
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
    isLoading: isLoadingFilters,
    warnings: filterWarnings,
    aip160,
  } = useChromeOSFilters(handleFilterChange);

  const { columns } = useRepairQueueColumns();

  const combinedWarnings = useMemo(
    () => filterWarnings || [],
    [filterWarnings],
  );

  return (
    <div
      css={{
        margin: '24px',
        paddingBottom: '40px',
      }}
    >
      <WarningNotifications warnings={combinedWarnings} />
      <Box
        sx={{
          padding: '16px 21px',
          border: `1px solid ${colors.grey[300]}`,
          borderRadius: 1,
          mb: 3,
        }}
      >
        <Typography variant="h4" component="h1">
          ChromeOS Repairs
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          View and monitor ChromeOS devices currently in the repair queue.
        </Typography>
      </Box>

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
          searchPlaceholder='Add a filter (e.g. "dut1" or "pool:DUT_POOL_QUOTA")'
        />
      </div>

      <div
        css={{
          marginTop: 24,
        }}
      >
        <ChromeOSRepairTable columns={columns} filter={aip160()} />
      </div>
    </div>
  );
};

export const ChromeOsRepairDashboard = ChromeOSRepairDashboard;

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-chromeos-repairs">
      <FleetHelmet pageTitle="ChromeOS Repairs" />
      <RecoverableErrorBoundary key="fleet-chromeos-repairs">
        <LoggedInBoundary>
          <ChromeOSRepairDashboard />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
