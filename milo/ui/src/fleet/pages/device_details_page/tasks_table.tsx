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

import { GridColDef } from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';

import { StyledGrid } from '@/fleet/components/data_table/styled_data_grid';
import {
  DEVICE_TASKS_MILO_HOST,
  DEVICE_TASKS_SWARMING_HOST,
  extractBuildUrlFromTagData,
} from '@/fleet/utils/builds';
import {
  StateQuery,
  TaskResultResponse,
  TaskState,
  taskStateToJSON,
  TasksWithPerfRequest,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

import { useDeviceData } from './use_device_data';

// Similar to Swarming's implementation in:
// https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/luci/appengine/swarming/ui2/modules/task-page/task-page-helpers.js;l=100;drc=6c1b10b83a339300fc10d5f5e08a56f1c48b3d3e
// TODO: Look into if we can make the UX for this prettier than what Swarming does.
const prettifySwarmingState = (task: TaskResultResponse): string => {
  if (task.state === TaskState.COMPLETED) {
    if (task.failure) {
      return 'COMPLETED (FAILURE)';
    }
    return 'COMPLETED (SUCCESS)';
  }
  return taskStateToJSON(task.state);
};

export const Tasks = ({
  id,
  swarmingHost = DEVICE_TASKS_SWARMING_HOST,
}: {
  id: string;
  swarmingHost?: string;
}) => {
  const device = useDeviceData(id);
  const swarmingCli = useTasksClient(swarmingHost);
  const dutId = device?.dutId;

  const taskData = useQuery({
    ...swarmingCli.ListTasks.query(
      TasksWithPerfRequest.fromPartial({
        tags: [`dut_id:${dutId}`],
        state: StateQuery.QUERY_ALL,
        limit: 20,
      }),
    ),
    refetchInterval: 60000,
  });

  const tasks = taskData?.data?.items || [];

  const taskMap = new Map(tasks.map((t) => [t.taskId, t]));

  const taskGridData = tasks.map((t) => ({
    id: t.taskId,
    task: t.name,
    started: t.startedTs,
    duration: `${t.duration}s`,
    result: prettifySwarmingState(t),
    tags: t.tags,
  }));

  // TODO: 371010330 - Prettify these columns.
  const columns: GridColDef[] = [
    // TODO: 393586616 - Add a link to the Milo UI.
    {
      field: 'task',
      headerName: 'Task',
      flex: 2,
      renderCell: (params) => {
        return (
          <>
            <a
              href={extractBuildUrlFromTagData(
                taskMap.get(`${params.id}`)?.tags || [],
                DEVICE_TASKS_MILO_HOST,
              )}
              target="_blank"
              rel="noreferrer"
            >
              {params.value}
            </a>
          </>
        );
      },
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
    // TODO: 371010330 - Make rows and add a failure icon somehwere (for a11y)
    // if result is a failure.
    {
      field: 'result',
      headerName: 'Result',
      flex: 1,
    },
  ];

  return (
    <StyledGrid
      rows={taskGridData}
      columns={columns}
      disableColumnMenu
      disableColumnFilter
      hideFooterPagination
    />
  );
};
