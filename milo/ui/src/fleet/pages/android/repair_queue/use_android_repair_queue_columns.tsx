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

import { Box, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import { AndroidDeviceIdCell } from '@/fleet/pages/device_list_page/android/android_device_id_cell';
import { getAndroidStatusColor } from '@/fleet/pages/device_list_page/android/android_state';
import { FC_CellProps } from '@/fleet/types/table';
import { getMobileHarnessHostListLink } from '@/fleet/utils/mobile_harness';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { AndroidRepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidMachineTypeLabel, isAndroidHost } from './android_machine_type';

export type AndroidRepairQueueRow = AndroidRepairQueueItem;
export type AndroidRepairQueueColumnDef = MRT_ColumnDef<AndroidRepairQueueRow>;

// Rows are keyed by `taskId` when present; unclaimed rows fall back to the
// composite `deviceId/machineType` key since a device and a host may share
// the same `deviceId`.
export const getAndroidRepairQueueRowId = (row: AndroidRepairQueueRow) =>
  row.taskId && row.taskId !== '0'
    ? row.taskId
    : `${row.deviceId}/${androidMachineTypeLabel(row.fcMachineType)}`;

// Column ids match the field names of the backend filter map
// (infra/fleetconsole/internal/core/android/filter_map.go) so they can be
// used as `order_by` / `filter` fields once sorting and filtering are enabled.
export const useAndroidRepairQueueColumns = (
  workspace: AndroidPageWorkspace,
) => {
  const columns: AndroidRepairQueueColumnDef[] = useMemo(() => {
    const allColumns: AndroidRepairQueueColumnDef[] = [
      {
        id: 'device_id',
        header: 'Id',
        accessorKey: 'deviceId',
        size: 100,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => (
          <AndroidDeviceIdCell
            deviceId={row.original.deviceId}
            workspace={workspace}
            mhLink={
              isAndroidHost(row.original.fcMachineType)
                ? getMobileHarnessHostListLink(row.original.deviceId)
                : undefined
            }
          />
        ),
      },
      {
        // Broken hosts are queued alongside devices; `device_id` is only
        // unique together with `fc_machine_type` (`device` | `host`).
        id: 'fc_machine_type',
        header: 'Machine Type',
        accessorFn: (row) => androidMachineTypeLabel(row.fcMachineType),
        size: 70,
        minSize: 60,
        maxSize: 200,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'lab_name',
        header: 'Lab',
        accessorKey: 'labName',
        size: 90,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'host_group',
        header: 'Host Group',
        accessorFn: (row) => labelValuesToString(row.hostGroups || []),
        size: 90,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'run_target',
        header: 'Run Target',
        accessorKey: 'runTarget',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'state',
        header: 'State',
        accessorKey: 'state',
        size: 80,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: renderChipCell<AndroidRepairQueueRow>({
          getValueOrUrl: () => workspaces[workspace].docsUrl,
          getColor: getAndroidStatusColor,
          getTrackingEvent: (value) => ({
            eventName: 'state_doc_link_clicked',
            payload: { componentName: 'android_dut_state', activeTab: value },
          }),
        }),
      },
      {
        id: 'host_group_run_target_health',
        header: 'Host Group / Run Target Health',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Header: () => (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <span>Host Group / Run Target Health</span>
            <InfoTooltip fontSize="1rem">
              Percentage of devices in the assigned host group(s) and run target
              that are healthy (not in manual repair). For devices with multiple
              host groups, the lowest percentage is shown.
            </InfoTooltip>
          </Box>
        ),
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => {
          const hostGroupHealth = row.original.hostGroupHealthPct;
          const runTargetHealth = row.original.runTargetHealthPct;
          const hostGroupStr =
            hostGroupHealth !== undefined && hostGroupHealth !== null
              ? `${Math.round(hostGroupHealth * 100)}%`
              : '-';
          const runTargetStr =
            runTargetHealth !== undefined && runTargetHealth !== null
              ? `${Math.round(runTargetHealth * 100)}%`
              : '-';
          return (
            <Typography
              variant="body2"
              sx={{ fontSize: '13px' }}
            >{`${hostGroupStr} / ${runTargetStr}`}</Typography>
          );
        },
      },
    ];
    if (workspace === 'Pixel') {
      return allColumns.filter((col) => col.id !== 'host_group');
    }
    return allColumns;
  }, [workspace]);

  return { columns };
};
