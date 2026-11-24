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
import { getRepairQueueRank } from '@/fleet/components/repair_queue/pinned_rows';
import { PriorityScoreCell } from '@/fleet/components/repair_queue/priority_score_cell';
import { RepairQueueRankCell } from '@/fleet/components/repair_queue/repair_queue_rank_cell';
import { useCurrentUserEmail } from '@/fleet/components/repair_queue/use_current_user_email';
import { parsePriorityRules } from '@/fleet/components/repair_queue/utils';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import { AndroidDeviceIdCell } from '@/fleet/pages/device_list_page/android/android_device_id_cell';
import { getAndroidStatusColor } from '@/fleet/pages/device_list_page/android/android_state';
import { FC_CellProps } from '@/fleet/types/table';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getMobileHarnessHostListLink } from '@/fleet/utils/mobile_harness';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { AndroidRepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidMachineTypeLabel, isAndroidHost } from './android_machine_type';
import { useAndroidPriorityRuleFilterBuilders } from './use_android_priority_rule_filter_builders';
import { FC_MACHINE_TYPE_FILTER_KEY } from './use_android_priority_rule_filter_builders';
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
  /** Zero-based index of the current page, used as a fallback when `row.rank` is unset. */
  pageIndex?: number;
  pageSize?: number;
  siteId?: string;
  pinnedRowIndices?: ReadonlySet<number>;
  onError?: (message: string) => void;
}

// Column ids match the field names of the backend filter map
// (infra/fleetconsole/internal/core/android/filter_map.go) so they can be
// used as `order_by` / `filter` fields once sorting and filtering are enabled.
export const useAndroidRepairQueueColumns = (
  workspace: AndroidPageWorkspace,
  {
    pageIndex = 0,
    pageSize = 100,
    siteId = '',
    pinnedRowIndices,
    onError,
  }: UseAndroidRepairQueueColumnsOptions = {},
) => {
  const { mutate: claimTask, isPending: isClaimPending } =
    useClaimAndroidRepairTask({
      onError: (err) => {
        onError?.(getErrorMessage(err, 'claim repair task'));
      },
    });
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimAndroidRepairTask({
      onError: (err) => {
        onError?.(getErrorMessage(err, 'unclaim repair task'));
      },
    });
  const isPending = isClaimPending || isUnclaimPending;

  const currentUser = useCurrentUserEmail();

  const { filterBuilders } = useAndroidPriorityRuleFilterBuilders(workspace);

  const { rules } = useAndroidPriorityRules(workspace, siteId);
  const rulesById = useMemo(
    () => parsePriorityRules(rules, filterBuilders),
    [rules, filterBuilders],
  );

  const columns: AndroidRepairQueueColumnDef[] = useMemo(() => {
    const allColumns: AndroidRepairQueueColumnDef[] = [
      {
        id: 'rank',
        header: 'Rank',
        size: 38,
        minSize: 38,
        maxSize: 76,
        grow: false,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<AndroidRepairQueueRow>) => (
          <RepairQueueRankCell
            rank={getRepairQueueRank(
              row.original.rank,
              pageIndex,
              pageSize,
              row.index,
            )}
            isPinned={Boolean(pinnedRowIndices?.has(row.index))}
          />
        ),
      },
      {
        id: 'device_id',
        header: 'ID',
        accessorKey: 'deviceId',
        size: 102,
        minSize: 88,
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
        id: FC_MACHINE_TYPE_FILTER_KEY,
        header: 'Machine Type',
        accessorFn: (row) => androidMachineTypeLabel(row.fcMachineType),
        size: 62,
        minSize: 58,
        maxSize: 200,
        grow: false,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'lab_name',
        header: 'Lab',
        accessorKey: 'labName',
        size: 34,
        minSize: 34,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>
            {cell.getValue<string>()?.trim() || '-'}
          </EllipsisTooltip>
        ),
      },
      {
        id: 'host_group',
        header: 'Host Group',
        accessorFn: (row) => labelValuesToString(row.hostGroups || []),
        size: 42,
        minSize: 40,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>
            {cell.getValue<string>()?.trim() || '-'}
          </EllipsisTooltip>
        ),
      },
      {
        id: 'run_target',
        header: 'Run Target',
        accessorKey: 'runTarget',
        size: 42,
        minSize: 40,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<AndroidRepairQueueRow>) => (
          <EllipsisTooltip>
            {cell.getValue<string>()?.trim() || '-'}
          </EllipsisTooltip>
        ),
      },
      {
        id: 'state',
        header: 'State',
        accessorKey: 'state',
        size: 91,
        minSize: 90,
        maxSize: 700,
        grow: false,
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
        size: 66,
        minSize: 64,
        maxSize: 700,
        grow: false,
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
            etag={row.original.etag}
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
        size: 58,
        minSize: 56,
        maxSize: 700,
        enableSorting: false,
        meta: {
          infoTooltip:
            'Percentage of healthy devices and hosts in the assigned host ' +
            'group(s) and run target. For devices with multiple host groups, ' +
            'the lowest percentage is shown.',
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
        size: 90,
        minSize: 84,
        maxSize: 180,
        grow: false,
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
    pinnedRowIndices,
    rulesById,
  ]);

  return { columns };
};
