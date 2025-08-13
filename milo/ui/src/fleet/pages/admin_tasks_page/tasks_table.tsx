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

import { Alert, AlertTitle } from '@mui/material';

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
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

const DEFAULT_PAGE_SIZE = 50;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const USER_TAG = 'client_user';

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
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useTasksClient(swarmingHost);
  const authState = useAuthState();
  const autorepairTags = authState.email
    ? [`${USER_TAG}:${authState.email}`]
    : [];
  const taskData = useTasks({
    client,
    tags: autorepairTags,
    limit: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
  });

  if (!authState.email) {
    return (
      <Alert severity="error">
        <AlertTitle>Not logged in.</AlertTitle>
        <p>Please login to see autorepair jobs.</p>
      </Alert>
    );
  }

  if (taskData.isError) {
    return (
      <Alert severity="error">
        {getErrorMessage(taskData.error, 'list tasks')}{' '}
      </Alert>
    );
  }

  if (taskData.isLoading) {
    return (
      <div
        css={{
          width: '100%',
          margin: '24px 0px',
        }}
      >
        <CentralizedProgress />
      </div>
    );
  }

  if (!taskData.tasks?.length) {
    return (
      <Alert severity="info">
        <AlertTitle>No recent autorepair jobs found.</AlertTitle>
        <p>No autorepair jobs triggered by you recently.</p>
      </Alert>
    );
  }
  return (
    <TasksGrid
      tasks={taskData.tasks}
      pagerCtx={pagerCtx}
      columnKeys={COLUMNS}
      nextPageToken={taskData.nextPageToken}
      swarmingHost={swarmingHost}
    />
  );
};
