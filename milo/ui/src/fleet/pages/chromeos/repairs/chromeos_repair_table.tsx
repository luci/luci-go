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

import { Alert, AlertTitle } from '@mui/material';
import { MaterialReactTable } from 'material-react-table';
import { useMemo } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import {
  FC_TableOptions,
  useFCDataTable,
} from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { getErrorMessage } from '@/fleet/utils/errors';
import { ListRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useRepairQueue } from './use_repair_queue';
import {
  RepairQueueColumnDef,
  RepairQueueRow,
  useRepairQueueColumns,
} from './use_repair_queue_columns';

const CHROMEOS_REPAIRS_LOCAL_STORAGE_KEY = 'fleet_chromeos_repairs_table';

interface ChromeOSRepairTableProps {
  columns?: RepairQueueColumnDef[];
  filter: string;
}

export const ChromeOSRepairTable = ({
  columns: propColumns,
  filter,
}: ChromeOSRepairTableProps) => {
  const { columns: defaultColumns } = useRepairQueueColumns();
  const columns = propColumns || defaultColumns;

  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100],
    defaultPageSize: 100,
  });

  const { pageSize, pageToken } = usePager(pagerCtx);

  const [sorting, onSortingChange, orderByParam] = useMrtSortingState(
    columns,
    pagerCtx,
  );

  const request = useMemo(
    () =>
      ListRepairQueueRequest.fromPartial({
        pageSize,
        pageToken,
        orderBy: orderByParam,
        filter,
      }),
    [pageSize, pageToken, orderByParam, filter],
  );

  const queueQuery = useRepairQueue(request);

  const {
    repairQueueItems = [],
    nextPageToken = '',
    totalSize = 0,
  } = queueQuery.data || {};

  const { columnSizing, onColumnSizingChange } = useMrtColumnSizing(
    CHROMEOS_REPAIRS_LOCAL_STORAGE_KEY,
  );

  const tableOptions: FC_TableOptions<RepairQueueRow> = useMemo(
    () => ({
      columns,
      data: repairQueueItems,
      enableRowSelection: false,
      positionToolbarAlertBanner: 'none',
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={totalSize}
          nextPageToken={nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      getRowId: (row: RepairQueueRow) => row.taskId,
      onSortingChange,
      enablePagination: false,
      enableTopToolbar: false,
      enableColumnVirtualization: process.env.NODE_ENV !== 'test',
      state: {
        sorting,
        columnSizing,
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
      onSortingChange,
      sorting,
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
      ? getErrorMessage(queueQuery.error, 'fetch repair queue')
      : undefined,
  });

  if (queueQuery.isError && !queueQuery.data) {
    return (
      <Alert severity="error">
        <AlertTitle>Error Loading Repair Queue</AlertTitle>
        {getErrorMessage(queueQuery.error, 'fetch repair queue')}
      </Alert>
    );
  }

  return <MaterialReactTable table={table} />;
};
