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

import { Alert, AlertTitle, Snackbar } from '@mui/material';
import { MaterialReactTable } from 'material-react-table';
import { useMemo, useState } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import {
  FC_TableOptions,
  useFCDataTable,
} from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { usePager } from '@/fleet/hooks/use_pager';
import { getErrorMessage } from '@/fleet/utils/errors';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { ListAndroidRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { toAndroidWorkspaceEnum } from './android_workspace';
import { useAndroidRepairQueue } from './use_android_repair_queue';
import {
  AndroidRepairQueueColumnDef,
  AndroidRepairQueueRow,
  getAndroidRepairQueueRowId,
  useAndroidRepairQueueColumns,
} from './use_android_repair_queue_columns';

const ANDROID_REPAIRS_LOCAL_STORAGE_KEY = 'fleet_android_repairs_table';

interface AndroidRepairQueueTableProps {
  workspace: AndroidPageWorkspace;
  columns?: AndroidRepairQueueColumnDef[];
  filter?: string;
}

export const AndroidRepairQueueTable = ({
  workspace,
  columns: propColumns,
  filter = '',
}: AndroidRepairQueueTableProps) => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100],
    defaultPageSize: 100,
  });

  const { pageSize, pageToken, pageIndex } = usePager(pagerCtx);

  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const { columns: defaultColumns } = useAndroidRepairQueueColumns(workspace, {
    pageIndex,
    pageSize,
    onError: setErrorMessage,
  });
  const columns = propColumns || defaultColumns;

  const request = useMemo(
    () =>
      ListAndroidRepairQueueRequest.fromPartial({
        pageSize,
        pageToken,
        orderBy: '',
        filter,
        workspace: toAndroidWorkspaceEnum(workspace),
      }),
    [pageSize, pageToken, filter, workspace],
  );

  const queueQuery = useAndroidRepairQueue(request);

  const {
    repairQueueItems = [],
    nextPageToken = '',
    totalSize = 0,
  } = queueQuery.data || {};

  const { columnSizing, onColumnSizingChange } = useMrtColumnSizing(
    ANDROID_REPAIRS_LOCAL_STORAGE_KEY,
  );

  const tableOptions: FC_TableOptions<AndroidRepairQueueRow> = useMemo(
    () => ({
      columns,
      data: repairQueueItems,
      enableRowSelection: false,
      enableSorting: false,
      enableColumnActions: false,
      enableHiding: false,
      defaultColumn: {
        enableColumnActions: false,
        enableColumnFilter: false,
        enableHiding: false,
      },
      positionToolbarAlertBanner: 'none',
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={totalSize}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      getRowId: getAndroidRepairQueueRowId,
      enablePagination: false,
      enableTopToolbar: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      state: {
        columnSizing,
        columnOrder: columns.map((col) => col.id || ''),
        isLoading: queueQuery.isPending && !queueQuery.isPlaceholderData,
        showProgressBars: queueQuery.isFetching,
      },
      manualFiltering: true,
      manualPagination: true,
      onColumnSizingChange,
    }),
    [
      columns,
      repairQueueItems,
      totalSize,
      nextPageToken,
      pagerCtx,
      columnSizing,
      queueQuery.isPending,
      queueQuery.isPlaceholderData,
      queueQuery.isFetching,
      onColumnSizingChange,
    ],
  );

  const table = useFCDataTable({
    ...tableOptions,
    error: queueQuery.error
      ? getErrorMessage(queueQuery.error, 'fetch android repair queue')
      : undefined,
  });

  if (queueQuery.isError && !queueQuery.data) {
    return (
      <Alert severity="error">
        <AlertTitle>Error Loading Android Repair Queue</AlertTitle>
        {getErrorMessage(queueQuery.error, 'fetch android repair queue')}
      </Alert>
    );
  }

  return (
    <>
      <MaterialReactTable table={table} />
      <Snackbar
        open={Boolean(errorMessage)}
        autoHideDuration={6000}
        onClose={() => setErrorMessage(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => setErrorMessage(null)}
          severity="error"
          sx={{ width: '100%' }}
        >
          {errorMessage}
        </Alert>
      </Snackbar>
    </>
  );
};
