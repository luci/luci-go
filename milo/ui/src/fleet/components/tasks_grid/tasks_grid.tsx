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

import { GridColDef } from '@mui/x-data-grid';

import {
  getCurrentPageIndex,
  getPageSize,
  PagerContext,
} from '@/common/components/params_pager';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { colors } from '@/fleet/theme/colors';
import { extractBuildUrlFromTagData } from '@/fleet/utils/builds';
import { prettyDateTime } from '@/fleet/utils/dates';
import {
  getRowClassName,
  getTaskDuration,
  getTaskTagValue,
  prettifySwarmingState,
} from '@/fleet/utils/task_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { TaskResultResponse } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const UNKNOWN_ROW_COUNT = -1;

export interface TasksGridProps {
  tasks: readonly TaskResultResponse[];
  pagerCtx: PagerContext;
  nextPageToken?: string;
  swarmingHost: string;
  miloHost?: string;
}

export const TasksGrid = ({
  tasks,
  pagerCtx,
  nextPageToken,
  swarmingHost,
  miloHost,
}: TasksGridProps) => {
  const [searchParams] = useSyncedSearchParams();

  const taskMap = new Map(tasks.map((t) => [t.taskId, t]));
  const taskGridData = tasks.map((t) => ({
    id: t.taskId,
    task: t.name,
    started: prettyDateTime(t.startedTs),
    duration: getTaskDuration(t),
    result: prettifySwarmingState(t),
    buildVersion: getTaskTagValue(t, 'build'),
  }));

  // TODO: 371010330 - Prettify these columns.
  const columns: GridColDef[] = [
    {
      field: 'task',
      headerName: 'Task',
      flex: 2,
      renderCell: (params) => {
        const task = taskMap.get(params.id.toString());
        let url: string | undefined;
        if (miloHost && task?.tags) {
          url = extractBuildUrlFromTagData(task.tags, miloHost);
        }

        return (
          <a
            href={url ?? getSwarmingTaskURL(swarmingHost, params.id.toString())}
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
          nextPageToken: nextPageToken,
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
