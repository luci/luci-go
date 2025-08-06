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

import { Alert, AlertTitle } from '@mui/material';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { TasksGrid } from '@/fleet/components/tasks_grid/tasks_grid';
import { useBot, useBotTasks } from '@/fleet/hooks/swarming_hooks';
import {
  DEVICE_TASKS_MILO_HOST,
  DEVICE_TASKS_SWARMING_HOST,
} from '@/fleet/utils/builds';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

const DEFAULT_PAGE_SIZE = 50;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export const Tasks = ({
  dutId,
  swarmingHost = DEVICE_TASKS_SWARMING_HOST,
}: {
  dutId: string;
  swarmingHost?: string;
}) => {
  const [searchParams] = useSyncedSearchParams();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useBotsClient(swarmingHost);
  const botData = useBot(client, dutId);
  const tasksData = useBotTasks({
    client,
    botId: botData.info?.botId || '',
    limit: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
  });

  // First, ensure we have a valid botId to work with.
  if (botData.isError) {
    return (
      <Alert severity="error">
        {getErrorMessage(botData.error, 'list bots')}{' '}
      </Alert>
    );
  }
  if (botData.isLoading) {
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
  if (!botData.botFound) {
    return (
      <AlertWithFeedback
        severity="warning"
        title="Bot not found!"
        bugErrorMessage={`Bot not found for device: ${dutId}`}
      >
        <p>
          Oh no! No bots were found for this device (<code>dut_id={dutId}</code>
          ).
        </p>
      </AlertWithFeedback>
    );
  }

  // Now, check the tasks request.
  if (tasksData.isError) {
    return (
      <Alert severity="error">
        {getErrorMessage(tasksData.error, 'list tasks')}{' '}
      </Alert>
    );
  }
  if (tasksData.isLoading) {
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
  if (!tasksData.tasks?.length) {
    return (
      <Alert severity="info">
        <AlertTitle>No tasks found</AlertTitle>
        <dl>
          <dt>DUT ID</dt>
          <dd>{dutId}</dd>
          <dt>Bot ID</dt>
          <dd>{botData.info?.botId}</dd>
        </dl>
      </Alert>
    );
  }
  return (
    <TasksGrid
      tasks={tasksData.tasks}
      pagerCtx={pagerCtx}
      nextPageToken={tasksData.nextPageToken}
      swarmingHost={swarmingHost}
      miloHost={DEVICE_TASKS_MILO_HOST}
    />
  );
};
