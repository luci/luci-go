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
import { MaterialReactTable, MRT_TableOptions } from 'material-react-table';
import { useMemo } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useMrtColumnSizing } from '@/fleet/hooks/use_mrt_column_sizing';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import {
  ChromeOSColumnDef,
  ChromeOSDevice,
} from '@/fleet/pages/device_list_page/chromeos/chromeos_types';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  Device,
  DeviceState,
  DeviceType,
  ListRepairQueueRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useRepairQueue } from './use_repair_queue';

const CHROMEOS_REPAIRS_LOCAL_STORAGE_KEY = 'fleet_chromeos_repairs_table';

interface ChromeOSRepairTableProps {
  columns: ChromeOSColumnDef[];
  filter: string;
}

export const ChromeOSRepairTable = ({
  columns,
  filter,
}: ChromeOSRepairTableProps) => {
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

  const rows: ChromeOSDevice[] = useMemo(() => {
    return repairQueueItems.map((item) => {
      const syntheticDevice: ChromeOSDevice = Device.fromPartial({
        id: item.dutId,
        dutId: item.dutId,
        state: DeviceState.DEVICE_STATE_AVAILABLE,
        type: DeviceType.DEVICE_TYPE_PHYSICAL,
        deviceSpec: {
          labels: {
            'label-pool': { values: item.pool ? [item.pool] : [] },
            'label-model': { values: item.model ? [item.model] : [] },
            dut_state: { values: item.state ? [item.state] : [] },
            dut_name: { values: item.dutId ? [item.dutId] : [] },
          },
        },
      });
      return syntheticDevice;
    });
  }, [repairQueueItems]);

  const { columnSizing, onColumnSizingChange } = useMrtColumnSizing(
    CHROMEOS_REPAIRS_LOCAL_STORAGE_KEY,
  );

  const tableOptions: MRT_TableOptions<ChromeOSDevice> = useMemo(
    () => ({
      columns,
      data: rows,
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
      getRowId: (row: ChromeOSDevice) => row.id || row.dutId,
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
      rows,
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

  if (queueQuery.isError) {
    return (
      <Alert severity="error">
        <AlertTitle>Error Loading Repair Queue</AlertTitle>
        {getErrorMessage(queueQuery.error, 'fetch repair queue')}
      </Alert>
    );
  }

  return <MaterialReactTable table={table} />;
};
