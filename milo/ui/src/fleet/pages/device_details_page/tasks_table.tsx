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
import { GridColDef, GridRowParams } from '@mui/x-data-grid';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import {
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { Pagination } from '@/fleet/components/device_table/pagination';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import {
  TASK_ONGOING_STATES,
  TASK_EXCEPTIONAL_STATES,
} from '@/fleet/constants/tasks';
import { useBot, useTasks } from '@/fleet/hooks/swarming_hooks';
import { colors } from '@/fleet/theme/colors';
import {
  DEVICE_TASKS_MILO_HOST,
  DEVICE_TASKS_SWARMING_HOST,
  extractBuildUrlFromTagData,
} from '@/fleet/utils/builds';
import { prettyDateTime, prettySeconds } from '@/fleet/utils/dates';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  TaskResultResponse,
  TaskState,
  taskStateToJSON,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

const UNKNOWN_ROW_COUNT = -1;
const DEFAULT_PAGE_SIZE = 50;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

// Similar to Swarming's implementation in:
// https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/luci/appengine/swarming/ui2/modules/task-page/task-page-helpers.js;l=100;drc=6c1b10b83a339300fc10d5f5e08a56f1c48b3d3e
// TODO: Look into if we can make the UX for this prettier than what Swarming does.
const prettifySwarmingState = (task: TaskResultResponse): string => {
  if (task.state === TaskState.COMPLETED) {
    if (task.failure) {
      return 'FAILURE';
    }
    return 'SUCCESS';
  }
  return taskStateToJSON(task.state);
};

// Similar to Swarming's implementation in:
// https://chromium.googlesource.com/infra/luci/luci-py/+/refs/heads/main/appengine/swarming/ui2/modules/task-list/task-list-helpers.js#480
const getRowClassName = (params: GridRowParams): string => {
  const result = params.row.result;
  if (result === 'FAILURE') {
    return 'row--failure';
  }
  if (TASK_ONGOING_STATES.has(result)) {
    return 'row--pending';
  }
  if (result === 'BOT_DIED') {
    return 'row--bot_died';
  }
  if (result === 'CLIENT_ERROR') {
    return 'row--client_error';
  }
  if (TASK_EXCEPTIONAL_STATES.has(result)) {
    return 'row--exception';
  }
  return '';
};

const getTaskDuration = (task: TaskResultResponse): string => {
  let duration = task.duration;
  // Running tasks have no duration set, so we can figure it out.
  if (!duration && task.state === TaskState.RUNNING && task.startedTs) {
    duration = (Date.now() - Date.parse(task.startedTs)) / 1000;
  }

  return prettySeconds(duration);
};

const getTaskTagValue = (
  task: TaskResultResponse,
  tagName: string,
): string | undefined => {
  for (const item of task.tags) {
    const [key, value] = item.split(':');
    if (key && key === tagName && value) {
      return value;
    }
  }

  return undefined;
};

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
  const tasksData = useTasks({
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

  const taskMap = new Map(tasksData.tasks.map((t) => [t.taskId, t]));
  const taskGridData = tasksData.tasks.map((t) => ({
    id: t.taskId,
    task: t.name,
    started: prettyDateTime(t.startedTs),
    duration: getTaskDuration(t),
    result: prettifySwarmingState(t),
    tags: t.tags,
    buildVersion: getTaskTagValue(t, 'build'),
  }));

  // TODO: 371010330 - Prettify these columns.
  const columns: GridColDef[] = [
    {
      field: 'task',
      headerName: 'Task',
      flex: 2,
      renderCell: (params) => {
        const buildUrl = extractBuildUrlFromTagData(
          taskMap.get(params.id.toString())?.tags || [],
          DEVICE_TASKS_MILO_HOST,
        );

        return (
          <a
            href={
              buildUrl ??
              getSwarmingTaskURL(
                DEVICE_TASKS_SWARMING_HOST,
                params.id.toString(),
              )
            }
            target="_blank"
            rel="noreferrer"
          >
            {params.value}
          </a>
        );
      },
    },
    {
      field: 'buildVersion',
      headerName: 'Build version',
      flex: 1,
    },
    // TODO: 371010330 - Make rows and add a failure icon somewhere (for a11y)
    // if result is a failure.
    {
      field: 'result',
      headerName: 'Result',
      flex: 1,
    },
    {
      field: 'started',
      headerName: 'Started',
      flex: 1,
    },
    {
      field: 'duration',
      headerName: 'Duration',
      flex: 1,
    },
  ];

  return (
    <StyledGrid
      rows={taskGridData}
      columns={columns}
      slots={{ pagination: Pagination }}
      slotProps={{
        pagination: {
          pagerCtx: pagerCtx,
          nextPageToken: tasksData.nextPageToken,
        },
      }}
      paginationMode="server"
      pageSizeOptions={pagerCtx.options.pageSizeOptions}
      paginationModel={{
        page: getCurrentPageIndex(pagerCtx),
        pageSize: getPageSize(pagerCtx, searchParams),
      }}
      rowCount={UNKNOWN_ROW_COUNT}
      disableColumnMenu
      disableColumnFilter
      disableRowSelectionOnClick
      getRowClassName={getRowClassName}
      sx={{
        '& .row--failure, .row--failure:hover': {
          backgroundColor: colors.red[100],
        },
        '& .row--pending, .row--pending:hover': {
          backgroundColor: colors.yellow[100],
        },
        '& .row--bot_died, .row--bot_died:hover': {
          backgroundColor: colors.grey[100],
        },
        '& .row--client_error, .row--client_error:hover': {
          backgroundColor: colors.orange[100],
        },
        '& .row--exception, .row--exception:hover': {
          backgroundColor: colors.purple[100],
        },
        '& .MuiDataGrid-row:hover': {
          filter: 'brightness(0.94)',
        },
      }}
    />
  );
};
