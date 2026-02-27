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

import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Snackbar from '@mui/material/Snackbar';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router';

import { TimeRangeSelector } from '@/common/components/time_range_selector';
import { useTopBarConfig } from '@/crystal_ball/components/layout/top_bar_context';
import { useGetDashboardState } from '@/crystal_ball/hooks/use_dashboard_state_api';
import { formatApiError } from '@/crystal_ball/utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * Empty dashboard page that displays the response of the get call.
 */
export function DashboardPage() {
  const { dashboardId } = useParams();

  const {
    data: dashboardState,
    isLoading,
    error,
  } = useGetDashboardState(
    {
      name: `dashboardStates/${dashboardId}`,
    },
    {
      enabled: !!dashboardId,
    },
  );

  const [toastOpen, setToastOpen] = useState(false);
  const [searchParams] = useSyncedSearchParams();
  const isFirstRender = useRef(true);

  const timeOption = searchParams.get('time_option');
  const startTimeParam = searchParams.get('start_time');
  const endTimeParam = searchParams.get('end_time');

  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false;
      return;
    }
    // Only show toast if it's not the initial mount and params change
    setToastOpen(true);
  }, [timeOption, startTimeParam, endTimeParam]);

  const handleCloseToast = () => {
    setToastOpen(false);
  };

  const topBarAction = useMemo(() => <TimeRangeSelector />, []);
  useTopBarConfig(dashboardState?.displayName || 'Loading...', topBarAction);

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">
          Failed to load dashboard: {formatApiError(error)}
        </Alert>
      </Box>
    );
  }

  if (!dashboardState) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="warning">Dashboard not found</Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      <pre style={{ whiteSpace: 'pre-wrap', wordWrap: 'break-word' }}>
        {JSON.stringify(dashboardState, null, 2)}
      </pre>
      <Snackbar
        open={toastOpen}
        autoHideDuration={4000}
        onClose={handleCloseToast}
        message="Feature under construction"
      />
    </Box>
  );
}

/**
 * Component export for Dashboard Page.
 */
export function Component() {
  return <DashboardPage />;
}
