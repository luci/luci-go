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

import { TablePagination } from '@mui/material';
import {
  MaterialReactTable,
  MRT_Cell,
  MRT_ColumnDef,
  MRT_Row,
  MRT_TableInstance,
} from 'material-react-table';
import { useMemo } from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  getPrevFullRowCount,
  nextPageTokenUpdater,
  PagerContext,
  pageSizeUpdater,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { getSwarmingTaskURL } from '@/common/tools/url_utils';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
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

export type TaskGridColumnKey =
  | 'task'
  | 'buildVersion'
  | 'result'
  | 'started'
  | 'duration'
  | 'dut_name';

const COLUMN_CONFIG: Record<
  TaskGridColumnKey,
  { header: string; size: number; grow?: number }
> = {
  task: { header: 'Task', size: 300, grow: 2 },
  buildVersion: { header: 'Build version', size: 150 },
  result: { header: 'Result', size: 100 },
  started: { header: 'Started', size: 250 },
  duration: { header: 'Duration', size: 150 },
  dut_name: { header: 'DUT Name', size: 200 },
};

const TaskCell = ({
  cell,
  row,
  table,
}: {
  cell: MRT_Cell<Record<string, unknown>, unknown>;
  row: MRT_Row<Record<string, unknown>>;
  table: MRT_TableInstance<Record<string, unknown>>;
}) => {
  const meta = table.options.meta as {
    taskMap: Map<string, TaskResultResponse>;
    swarmingHost: string;
    miloHost?: string;
  };
  const taskId = row.original.id as string;
  const task = meta.taskMap.get(taskId);
  let url: string | undefined;
  if (meta.miloHost && task?.tags) {
    url = extractBuildUrlFromTagData(task.tags, meta.miloHost);
  }

  return (
    <EllipsisTooltip>
      <a
        href={url ?? getSwarmingTaskURL(meta.swarmingHost, taskId)}
        target="_blank"
        rel="noreferrer"
      >
        {cell.getValue<string>()}
      </a>
    </EllipsisTooltip>
  );
};

const DutNameCell = ({
  cell,
}: {
  cell: MRT_Cell<Record<string, unknown>, unknown>;
}) => {
  const dutName = cell.getValue<string>();
  if (!dutName) {
    return null;
  }
  return (
    <EllipsisTooltip>
      <a href={generateChromeOsDeviceDetailsURL(dutName)}>{dutName}</a>
    </EllipsisTooltip>
  );
};

export interface TasksGridProps {
  tasks: readonly TaskResultResponse[];
  pagerCtx: PagerContext;
  columnKeys: readonly TaskGridColumnKey[];
  nextPageToken?: string;
  swarmingHost: string;
  miloHost?: string;
}

export const TasksGrid = ({
  tasks,
  pagerCtx,
  columnKeys,
  nextPageToken,
  swarmingHost,
  miloHost,
}: TasksGridProps) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const taskMap = useMemo(
    () => new Map(tasks.map((t) => [t.taskId, t])),
    [tasks],
  );

  const taskGridData = useMemo(
    () =>
      tasks.map((t) => ({
        id: t.taskId,
        result: prettifySwarmingState(t),
        task: t.name,
        buildVersion: getTaskTagValue(t, 'build'),
        started: prettyDateTime(t.startedTs),
        duration: getTaskDuration(t),
        dut_name: getTaskTagValue(t, 'dut-name'),
      })),
    [tasks],
  );

  const columns = useMemo<MRT_ColumnDef<Record<string, unknown>>[]>(
    () =>
      columnKeys.map((key) => {
        const config = COLUMN_CONFIG[key];
        const colDef: MRT_ColumnDef<Record<string, unknown>> = {
          accessorKey: key,
          header: config.header,
          size: config.size,
          grow: config.grow,
        };

        if (key === 'task') {
          colDef.Cell = TaskCell;
        } else if (key === 'dut_name') {
          colDef.Cell = DutNameCell;
        }
        return colDef;
      }),
    [columnKeys],
  );

  const table = useFCDataTable({
    columns,
    data: taskGridData,
    meta: {
      taskMap,
      swarmingHost,
      miloHost,
    },
    enablePagination: false,
    enableColumnActions: false,
    enableColumnFilters: false,
    enableSorting: false,
    enableTopToolbar: true,
    enableStickyHeader: true,
    muiTableHeadRowProps: {
      sx: {
        minHeight: 'unset',
        '& .MuiTableCell-head:hover': {
          backgroundColor: `${colors.grey[100]} !important`,
        },
      },
    },
    muiTableBodyRowProps: (params: {
      row: MRT_Row<Record<string, unknown>>;
    }) => ({
      className: getRowClassName({
        result: params.row.original.result as string,
      }),
    }),
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        maxHeight: '80vh',
        '--cell-padding-horizontal': '16px',
        '& .Mui-TableHeadCell-Content': {
          minHeight: 'unset !important',
        },
        '& .row--failure, .row--failure:hover': {
          backgroundColor: `${colors.red[100]} !important`,
        },
        '& .row--pending, .row--pending:hover': {
          backgroundColor: `${colors.yellow[100]} !important`,
        },
        '& .row--bot_died, .row--bot_died:hover': {
          backgroundColor: `${colors.grey[100]} !important`,
        },
        '& .row--client_error, .row--client_error:hover': {
          backgroundColor: `${colors.orange[100]} !important`,
        },
        '& .row--exception, .row--exception:hover': {
          backgroundColor: `${colors.purple[100]} !important`,
        },
      },
    },
  });

  const currentPage = getCurrentPageIndex(pagerCtx);
  const pageSize = getPageSize(pagerCtx, searchParams);

  return (
    <>
      <MaterialReactTable table={table} />
      <TablePagination
        component="div"
        count={tasks.length === 0 && !nextPageToken ? 0 : -1}
        page={currentPage}
        rowsPerPage={pageSize}
        onPageChange={(_, page) => {
          const isPrevPage = page < currentPage;
          const isNextPage = page > currentPage;

          if (isPrevPage) {
            setSearchParams(prevPageTokenUpdater(pagerCtx));
          } else if (isNextPage && nextPageToken) {
            setSearchParams(nextPageTokenUpdater(pagerCtx, nextPageToken));
          }
        }}
        onRowsPerPageChange={(e) => {
          setSearchParams(pageSizeUpdater(pagerCtx, Number(e.target.value)));
        }}
        rowsPerPageOptions={pagerCtx.options.pageSizeOptions}
        labelDisplayedRows={() => {
          const realFrom = getPrevFullRowCount(pagerCtx) + 1;
          const realTo = realFrom + tasks.length - 1;
          const hasNextPage = !!nextPageToken;
          return `${realFrom}-${realTo} of ${hasNextPage ? `more than ${realTo}` : realTo}`;
        }}
      />
    </>
  );
};
