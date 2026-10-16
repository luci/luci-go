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

import { Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { AssigneeCell } from '@/fleet/components/repair_queue/assignee_cell';
import { PriorityScoreCell } from '@/fleet/components/repair_queue/priority_score_cell';
import { useCurrentUserEmail } from '@/fleet/components/repair_queue/use_current_user_email';
import { parsePriorityRules } from '@/fleet/components/repair_queue/utils';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import { AndroidDeviceIdCell } from '@/fleet/pages/device_list_page/android/android_device_id_cell';
import { getAndroidStatusColor } from '@/fleet/pages/device_list_page/android/android_state';
import { FC_CellProps } from '@/fleet/types/table';
import { getMobileHarnessHostListLink } from '@/fleet/utils/mobile_harness';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { AndroidRepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidMachineTypeLabel, isAndroidHost } from './android_machine_type';
import { useAndroidPriorityRuleFilterBuilders } from './use_android_priority_rule_filter_builders';
import { useAndroidPriorityRules } from './use_android_priority_rules';
import {
  useClaimAndroidRepairTask,
  useUnclaimAndroidRepairTask,
} from './use_claim_android_repair_task';

export type AndroidRepairQueueRow = AndroidRepairQueueItem;
export type AndroidRepairQueueColumnDef = MRT_ColumnDef<AndroidRepairQueueRow>;

// Rows are keyed by `taskId` when present; unclaimed rows fall back to the
// composite `deviceId/machineType` key since a device and a host may share
// the same `deviceId`.
export const getAndroidRepairQueueRowId = (row: AndroidRepairQueueRow) =>
  row.taskId && row.taskId !== '0'
    ? row.taskId
    : `${row.deviceId}/${androidMachineTypeLabel(row.fcMachineType)}`;

export interface UseAndroidRepairQueueColumnsOptions {
  /** Zero-based index of the current page, used to compute the Rank. */
  pageIndex?: number;
  pageSize?: number;
}

// Column ids match the field names of the backend filter map
// (infra/fleetconsole/internal/core/android/filter_map.go) so they can be
// used as `order_by` / `filter` fields once sorting and filtering are enabled.
export const useAndroidRepairQueueColumns = (
  workspace: AndroidPageWorkspace,
  { pageIndex = 0, pageSize = 100 }: UseAndroidRepairQueueColumnsOptions = {},
) => {
  const { mutate: claimTask, isPending: isClaimPending } =
    useClaimAndroidRepairTask();
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimAndroidRepairTask();
  const isPending = isClaimPending || isUnclaimPending;

  const currentUser = useCurrentUserEmail();

  const { filterBuilders } = useAndroidPriorityRuleFilterBuilders(workspace);

  const { rules } = useAndroidPriorityRules(workspace);
  const rulesById = useMemo(
    () => parsePriorityRules(rules, filterBuilders),
    [rules, filterBuilders],
  );

  const columns: AndroidRepairQueueColumnDef[] = useMemo(() => {
    const allColumns: AndroidRepairQueueColumnDef[] = [
      {
        id: 'rank',
        header: 'Rank',
        size: 25,
        minSize: 25,
        maxSize: 60,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => {
          return String(pageIndex * pageSize + row.index + 1);
        },
      },
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
        id: 'assignee',
        header: 'Assignee',
        size: 70,
        minSize: 60,
        maxSize: 700,
        enableSorting: false,
        muiTableHeadCellProps: {
          align: 'center',
        },
        muiTableBodyCellProps: {
          align: 'center',
        },
        accessorFn: (row) => row.claimedBy ?? '',
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => (
          <AssigneeCell
            claimedBy={row.original.claimedBy}
            taskId={row.original.taskId}
            currentUser={currentUser}
            isPending={isPending}
            claimTask={claimTask}
            unclaimTask={unclaimTask}
          />
        ),
      },
      {
        id: 'host_group_run_target_health',
        header: 'Host Group / Run Target Health',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        meta: {
          infoTooltip:
            'Percentage of devices in the assigned host group(s) and run target ' +
            'that are healthy (not in manual repair). For devices with multiple ' +
            'host groups, the lowest percentage is shown.',
        },
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
              noWrap
              sx={{ fontSize: '13px' }}
            >{`${hostGroupStr} / ${runTargetStr}`}</Typography>
          );
        },
      },
      {
        id: 'priority_score',
        header: 'Priority Score',
        size: 65,
        minSize: 60,
        maxSize: 180,
        enableSorting: false,
        meta: {
          infoTooltip:
            'Devices are ranked in real time by summing active rule weights. Higher score = higher priority.',
        },
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => (
          <PriorityScoreCell item={row.original} rulesById={rulesById} />
        ),
      },
    ];
    if (workspace === 'Pixel') {
      return allColumns.filter((col) => col.id !== 'host_group');
    }
    return allColumns;
  }, [
    claimTask,
    unclaimTask,
    currentUser,
    isPending,
    workspace,
    pageIndex,
    pageSize,
    rulesById,
  ]);

  return { columns };
};
