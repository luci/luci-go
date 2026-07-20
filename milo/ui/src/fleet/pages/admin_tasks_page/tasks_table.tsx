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

import { Alert, AlertTitle, Box, Typography } from '@mui/material';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { useAuthState } from '@/common/components/auth_state_provider';
import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import {
  TasksGrid,
  TaskGridColumnKey,
} from '@/fleet/components/tasks_grid/tasks_grid';
import { useTasks } from '@/fleet/hooks/swarming_hooks';
import {
  useAdminTaskTags,
  usePendingAdminTasks,
} from '@/fleet/hooks/use_pending_admin_tasks';
import {
  DEVICE_TASKS_MILO_HOST,
  DEVICE_TASKS_SWARMING_HOST,
} from '@/fleet/utils/builds';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { StateQuery } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

const DEFAULT_PAGE_SIZE = 50;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

const COLUMNS: readonly TaskGridColumnKey[] = [
  'task',
  'dut_name',
  'result',
  'started',
  'duration',
];

export const TasksTable = ({
  swarmingHost = DEVICE_TASKS_SWARMING_HOST,
}: {
  swarmingHost?: string;
}) => {
  const [searchParams] = useSyncedSearchParams();
  const activePagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
    pageSizeKey: 'active_limit',
    pageTokenKey: 'active_cursor',
  });

  const historyPagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
    pageSizeKey: 'history_limit',
    pageTokenKey: 'history_cursor',
  });

  const client = useTasksClient(swarmingHost);
  const authState = useAuthState();
  const adminTaskTags = useAdminTaskTags();

  const activeTaskData = usePendingAdminTasks({
    swarmingHost,
    limit: getPageSize(activePagerCtx, searchParams),
    pageToken: getPageToken(activePagerCtx, searchParams),
  });

  const historyTaskData = useTasks(
    {
      client,
      tags: adminTaskTags,
      limit: getPageSize(historyPagerCtx, searchParams),
      pageToken: getPageToken(historyPagerCtx, searchParams),
      state: StateQuery.QUERY_ALL,
    },
    { enabled: adminTaskTags.length > 0 },
  );

  if (!authState.email) {
    return (
      <Alert severity="error">
        <AlertTitle>Not logged in.</AlertTitle>
        <p>Please login to see admin tasks.</p>
      </Alert>
    );
  }

  const renderActiveTasksSection = () => {
    if (activeTaskData.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(activeTaskData.error, 'list active tasks')}
        </Alert>
      );
    }
    if (activeTaskData.isLoading) {
      return (
        <div css={{ width: '100%', margin: '24px 0px' }}>
          <CentralizedProgress />
        </div>
      );
    }

    const tasks = activeTaskData.tasks || [];
    if (tasks.length === 0 && !activeTaskData.nextPageToken) {
      return (
        <Alert severity="info">
          No active tasks currently running or pending.
        </Alert>
      );
    }

    return (
      <TasksGrid
        tasks={tasks}
        pagerCtx={activePagerCtx}
        columnKeys={COLUMNS}
        nextPageToken={activeTaskData.nextPageToken}
        swarmingHost={swarmingHost}
        miloHost={DEVICE_TASKS_MILO_HOST}
      />
    );
  };

  const renderHistoryTasksSection = () => {
    if (historyTaskData.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(historyTaskData.error, 'list task history')}
        </Alert>
      );
    }
    if (historyTaskData.isLoading) {
      return (
        <div css={{ width: '100%', margin: '24px 0px' }}>
          <CentralizedProgress />
        </div>
      );
    }

    const tasks = historyTaskData.tasks || [];
    if (tasks.length === 0 && !historyTaskData.nextPageToken) {
      return <Alert severity="info">No completed task history found.</Alert>;
    }

    return (
      <TasksGrid
        tasks={tasks}
        pagerCtx={historyPagerCtx}
        columnKeys={COLUMNS}
        nextPageToken={historyTaskData.nextPageToken}
        swarmingHost={swarmingHost}
        miloHost={DEVICE_TASKS_MILO_HOST}
      />
    );
  };

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 4, mt: 3 }}>
      <Box>
        <Typography variant="h5" sx={{ mb: 1 }}>
          Active Tasks
        </Typography>
        {renderActiveTasksSection()}
      </Box>
      <Box>
        <Typography variant="h5" sx={{ mb: 1 }}>
          Task History
        </Typography>
        {renderHistoryTasksSection()}
      </Box>
    </Box>
  );
};
