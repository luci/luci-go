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

import { Box, Chip, Link, TablePagination, Typography } from '@mui/material';
import {
  MaterialReactTable,
  MRT_Cell,
  MRT_ColumnDef,
  MRT_Row,
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
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { colors } from '@/fleet/theme/colors';
import { prettyDateTime, prettySeconds } from '@/fleet/utils/dates';
import { getRowClassName } from '@/fleet/utils/task_utils';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  TaskHistoryItem,
  TaskSource,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface UnifiedTasksGridProps {
  tasks: readonly TaskHistoryItem[];
  pagerCtx: PagerContext;
  nextPageToken?: string;
}

interface UnifiedGridRow {
  id: string;
  source: TaskSource;
  task: string;
  buildVersion: string;
  result: string;
  started: string;
  duration: string;
  taskUrl: string;
}

const UNIFIED_FAILURE_RESULTS = new Set([
  'FAIL',
  'FAILED',
  'ERROR',
  'INFRA_ERROR',
  'ALLOC_ERROR',
  'ALLOC_FAIL',
  'TIMEOUT',
]);

const formatSource = (source: TaskSource): string => {
  switch (source) {
    case TaskSource.TASK_SOURCE_MOBILE_HARNESS:
      return 'Mobile Harness';
    case TaskSource.TASK_SOURCE_SWARMING:
      return 'Swarming';
    default:
      return 'Unknown';
  }
};

const formatDuration = (task: TaskHistoryItem): string => {
  if (task.state === 'RUNNING' && task.startTime) {
    const startTimeMs = Date.parse(task.startTime);
    if (!isNaN(startTimeMs)) {
      // Computed as a snapshot at render/refetch time; clamp to 0 to guard against clock skew.
      return prettySeconds(Math.max(0, (Date.now() - startTimeMs) / 1000));
    }
  }
  if (task.duration?.seconds) {
    const secs = Number(task.duration.seconds);
    if (!isNaN(secs)) {
      return prettySeconds(secs);
    }
  }
  return prettySeconds(0);
};

const getSourceColors = (source: TaskSource) => {
  switch (source) {
    case TaskSource.TASK_SOURCE_MOBILE_HARNESS:
      return {
        borderColor: colors.green[700],
        color: colors.green[900],
        backgroundColor: colors.green[50],
      };
    case TaskSource.TASK_SOURCE_SWARMING:
      return {
        borderColor: colors.blue[700],
        color: colors.blue[900],
        backgroundColor: colors.blue[50],
      };
    default:
      return {
        borderColor: colors.grey[700],
        color: colors.grey[900],
        backgroundColor: colors.grey[100],
      };
  }
};

const SourceCell = ({ cell }: { cell: MRT_Cell<UnifiedGridRow, unknown> }) => {
  const source = cell.getValue<TaskSource>();

  return (
    <Chip
      label={formatSource(source)}
      size="small"
      variant="outlined"
      sx={{
        ...getSourceColors(source),
        fontWeight: 600,
        fontSize: '0.75rem',
      }}
    />
  );
};

const TaskCell = ({
  cell,
  row,
}: {
  cell: MRT_Cell<UnifiedGridRow, unknown>;
  row: MRT_Row<UnifiedGridRow>;
}) => {
  const taskName = cell.getValue<string>();
  const { taskUrl } = row.original;
  const { trackEvent } = useGoogleAnalytics();

  if (!taskUrl) {
    return (
      <EllipsisTooltip tooltip={taskName}>
        <span>{taskName}</span>
      </EllipsisTooltip>
    );
  }

  return (
    <EllipsisTooltip tooltip={taskName}>
      <Link
        href={taskUrl}
        target="_blank"
        rel="noreferrer"
        onClick={() => {
          trackEvent('unified_task_link_clicked', {
            componentName: 'UnifiedTasksGrid',
          });
        }}
      >
        {taskName}
      </Link>
    </EllipsisTooltip>
  );
};

export const UnifiedTaskDetailPanel = ({
  row,
}: {
  row: MRT_Row<UnifiedGridRow>;
}) => {
  const {
    id: taskId,
    source,
    started: startTime,
    duration,
    result,
    buildVersion,
    taskUrl,
  } = row.original;
  const { trackEvent } = useGoogleAnalytics();

  return (
    <Box sx={{ p: 2, backgroundColor: colors.white }}>
      <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
        Task Details
      </Typography>
      <Box
        component="dl"
        sx={{
          display: 'grid',
          gridTemplateColumns: '160px 1fr',
          rowGap: 1,
          columnGap: 2,
          m: 0,
        }}
      >
        <Typography
          component="dt"
          sx={{ fontWeight: 500, color: colors.grey[700] }}
        >
          Task ID
        </Typography>
        <Typography component="dd" sx={{ m: 0 }}>
          {taskId}
        </Typography>

        <Typography
          component="dt"
          sx={{ fontWeight: 500, color: colors.grey[700] }}
        >
          Source System
        </Typography>
        <Typography component="dd" sx={{ m: 0 }}>
          {formatSource(source)}
        </Typography>

        <Typography
          component="dt"
          sx={{ fontWeight: 500, color: colors.grey[700] }}
        >
          Status
        </Typography>
        <Typography component="dd" sx={{ m: 0 }}>
          {result}
        </Typography>

        <Typography
          component="dt"
          sx={{ fontWeight: 500, color: colors.grey[700] }}
        >
          Start Time
        </Typography>
        <Typography component="dd" sx={{ m: 0 }}>
          {startTime || 'N/A'}
        </Typography>

        <Typography
          component="dt"
          sx={{ fontWeight: 500, color: colors.grey[700] }}
        >
          Duration
        </Typography>
        <Typography component="dd" sx={{ m: 0 }}>
          {duration}
        </Typography>

        {buildVersion && (
          <>
            <Typography
              component="dt"
              sx={{ fontWeight: 500, color: colors.grey[700] }}
            >
              Build Version
            </Typography>
            <Typography component="dd" sx={{ m: 0 }}>
              {buildVersion}
            </Typography>
          </>
        )}

        {taskUrl && (
          <>
            <Typography
              component="dt"
              sx={{ fontWeight: 500, color: colors.grey[700] }}
            >
              Deep Link
            </Typography>
            <Typography component="dd" sx={{ m: 0 }}>
              <Link
                href={taskUrl}
                target="_blank"
                rel="noreferrer"
                onClick={() => {
                  trackEvent('unified_task_link_clicked', {
                    componentName: 'UnifiedTaskDetailPanel',
                  });
                }}
              >
                {source === TaskSource.TASK_SOURCE_MOBILE_HARNESS
                  ? 'View in Mobile Harness FE'
                  : source === TaskSource.TASK_SOURCE_SWARMING
                    ? 'View in Swarming'
                    : 'View Task'}
              </Link>
            </Typography>
          </>
        )}
      </Box>
    </Box>
  );
};

const renderUnifiedTaskDetailPanel = ({
  row,
}: {
  row: MRT_Row<UnifiedGridRow>;
}) => <UnifiedTaskDetailPanel row={row} />;

export const UnifiedTasksGrid = ({
  tasks,
  pagerCtx,
  nextPageToken,
}: UnifiedTasksGridProps) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const gridData = useMemo<UnifiedGridRow[]>(
    () =>
      tasks.map((t) => ({
        id: t.taskId,
        source: t.source,
        task: t.name,
        buildVersion: t.buildVersion,
        result: t.state,
        started: prettyDateTime(t.startTime),
        duration: formatDuration(t),
        taskUrl: t.taskUrl,
      })),
    [tasks],
  );

  const columns = useMemo<MRT_ColumnDef<UnifiedGridRow>[]>(
    () => [
      {
        accessorKey: 'source',
        header: 'Source',
        size: 130,
        Cell: SourceCell,
      },
      {
        accessorKey: 'task',
        header: 'Task',
        size: 300,
        grow: 2,
        Cell: TaskCell,
      },
      {
        accessorKey: 'buildVersion',
        header: 'Build version',
        size: 150,
      },
      {
        accessorKey: 'result',
        header: 'Result',
        size: 100,
      },
      {
        accessorKey: 'started',
        header: 'Started',
        size: 250,
      },
      {
        accessorKey: 'duration',
        header: 'Duration',
        size: 150,
      },
    ],
    [],
  );

  const table = useFCDataTable({
    columns,
    data: gridData,
    enablePagination: false,
    enableColumnActions: false,
    enableColumnFilters: false,
    enableSorting: false,
    enableTopToolbar: true,
    enableBottomToolbar: false,
    enableStickyHeader: true,
    renderDetailPanel: renderUnifiedTaskDetailPanel,
    muiDetailPanelProps: {
      sx: {
        width: '100%',
        backgroundColor: `${colors.white} !important`,
      },
    },
    displayColumnDefOptions: {
      'mrt-row-expand': {
        size: 100,
        grow: false,
      },
    },
    muiTableHeadRowProps: {
      sx: {
        minHeight: 'unset',
        '& .MuiTableCell-head:hover': {
          backgroundColor: `${colors.grey[100]} !important`,
        },
      },
    },
    muiTableBodyRowProps: (params: { row: MRT_Row<UnifiedGridRow> }) => {
      const { result } = params.row.original;
      return {
        className: UNIFIED_FAILURE_RESULTS.has(result)
          ? 'row--failure'
          : getRowClassName({ result }),
      };
    },
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
        count={
          nextPageToken ? -1 : getPrevFullRowCount(pagerCtx) + tasks.length
        }
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
          if (tasks.length === 0) {
            return '0 of 0';
          }
          const realFrom = getPrevFullRowCount(pagerCtx) + 1;
          const realTo = realFrom + tasks.length - 1;
          const hasNextPage = !!nextPageToken;
          return `${realFrom}-${realTo} of ${hasNextPage ? `more than ${realTo}` : realTo}`;
        }}
      />
    </>
  );
};
