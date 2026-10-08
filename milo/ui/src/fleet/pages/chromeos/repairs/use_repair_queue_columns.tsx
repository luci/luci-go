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

import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import RemoveIcon from '@mui/icons-material/Remove';
import WarningIcon from '@mui/icons-material/Warning';
import { Box, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { AssigneeCell } from '@/fleet/components/repair_queue/assignee_cell';
import { PriorityScoreCell } from '@/fleet/components/repair_queue/priority_score_cell';
import { DutStateCell } from '@/fleet/pages/device_list_page/chromeos/dut_state_cell';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';
import {
  PriorityRule,
  RepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { PeripheralsCell } from './peripheral_state_indicator';
import {
  useClaimRepairTask,
  useUnclaimRepairTask,
} from './use_claim_repair_task';
import { usePriorityRules } from './use_priority_rules';

export type RepairQueueRow = RepairQueueItem;
export type RepairQueueColumnDef = MRT_ColumnDef<RepairQueueRow>;

export const REPAIR_QUEUE_COLUMN_IDS = [
  'rank',
  'dut_id',
  'label-pool',
  'label-model',
  'dut_state',
  'assignee',
  'peripherals',
  'pool_model_health',
  'priority_score',
] as const;

export interface UseRepairQueueColumnsOptions {
  pageIndex?: number;
  pageSize?: number;
}

export const useRepairQueueColumns = ({
  pageIndex = 0,
  pageSize = 100,
}: UseRepairQueueColumnsOptions = {}) => {
  const { mutate: claimTask, isPending: isClaimPending } = useClaimRepairTask();
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimRepairTask();
  const isPending = isClaimPending || isUnclaimPending;

  const authState = useAuthState();
  const currentUser = (authState.email || authState.identity || '').trim();

  const { rules } = usePriorityRules();
  const rulesById = useMemo(() => {
    const map = new Map<string, PriorityRule>();
    for (const r of rules) {
      map.set(r.id, r);
    }
    return map;
  }, [rules]);

  const columns: RepairQueueColumnDef[] = useMemo(() => {
    return [
      {
        id: 'rank',
        header: 'Rank',
        size: 25,
        minSize: 25,
        maxSize: 60,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          return String(pageIndex * pageSize + row.index + 1);
        },
      },
      {
        id: 'dut_id',
        header: 'Dut ID',
        accessorKey: 'dutId',
        size: 100,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'label-pool',
        header: 'Pool',
        accessorFn: (row) => labelValuesToString(row.pools || []),
        size: 90,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'label-model',
        header: 'Model',
        accessorKey: 'model',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'dut_state',
        header: 'State',
        accessorKey: 'state',
        size: 80,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <DutStateCell state={cell.getValue<string>()} />
        ),
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
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => (
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
        id: 'peripherals',
        header: 'Peripherals (W / B / S)',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Header: () => (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <span>Peripherals (W / B / S)</span>
            <InfoTooltip paperCss={{ maxWidth: 360, p: 2 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
                Peripheral Health States
              </Typography>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: 1.5 }}
              >
                Tracks Wi-Fi (W), Bluetooth (B), and Servo (S) status:
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 1,
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <CheckCircleIcon
                    sx={{ color: colors.green[500], fontSize: 18 }}
                  />
                  <Typography variant="body2">
                    <strong>OK</strong> &mdash; Healthy &amp; responsive
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <WarningIcon sx={{ color: colors.red[600], fontSize: 18 }} />
                  <Typography variant="body2">
                    <strong>Broken</strong> &mdash; Hardware failure
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <WarningIcon
                    sx={{ color: colors.orange[600], fontSize: 18 }}
                  />
                  <Typography variant="body2">
                    <strong>Missing</strong> &mdash; Disconnected / unplugged
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <RemoveIcon sx={{ color: colors.grey[500], fontSize: 18 }} />
                  <Typography variant="body2">
                    <strong>N/A</strong> &mdash; Not applicable to this DUT
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <HelpOutlineIcon
                    sx={{ color: colors.grey[500], fontSize: 18 }}
                  />
                  <Typography variant="body2">
                    <strong>Unknown</strong> &mdash; State indeterminate /
                    unrecorded
                  </Typography>
                </Box>
              </Box>
            </InfoTooltip>
          </Box>
        ),
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => (
          <PeripheralsCell
            wifiState={row.original.wifiState}
            bluetoothState={row.original.bluetoothState}
            servoState={row.original.servoState}
          />
        ),
      },
      {
        id: 'pool_model_health',
        header: 'Pool / Model Health',
        size: 85,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Header: () => (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <span>Pool / Model Health</span>
            <InfoTooltip fontSize="1rem">
              Percentage of devices in the assigned pool(s) and model(s) that
              are healthy (not in manual repair or repair failed). For devices
              with multiple pools or models, the lowest percentage is shown.
            </InfoTooltip>
          </Box>
        ),
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          const poolHealth = row.original.poolHealthPct;
          const modelHealth = row.original.modelHealthPct;
          const poolStr =
            poolHealth !== undefined && poolHealth !== null
              ? `${Math.round(poolHealth * 100)}%`
              : '-';
          const modelStr =
            modelHealth !== undefined && modelHealth !== null
              ? `${Math.round(modelHealth * 100)}%`
              : '-';
          return (
            <Typography
              variant="body2"
              sx={{ fontSize: '13px' }}
            >{`${poolStr} / ${modelStr}`}</Typography>
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
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => (
          <PriorityScoreCell item={row.original} rulesById={rulesById} />
        ),
      },
    ];
  }, [
    claimTask,
    unclaimTask,
    currentUser,
    isPending,
    pageIndex,
    pageSize,
    rulesById,
  ]);

  return { columns };
};
