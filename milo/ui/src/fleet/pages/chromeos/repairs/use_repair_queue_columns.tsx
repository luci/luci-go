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
import { Avatar, Box, Button, Link, Tooltip, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { DutStateCell } from '@/fleet/pages/device_list_page/chromeos/dut_state_cell';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';
import { RepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { PeripheralsCell } from './peripheral_state_indicator';
import {
  useClaimRepairTask,
  useUnclaimRepairTask,
} from './use_claim_repair_task';

export type RepairQueueRow = RepairQueueItem;
export type RepairQueueColumnDef = MRT_ColumnDef<RepairQueueRow>;

export const REPAIR_QUEUE_COLUMN_IDS = [
  'dut_id',
  'label-pool',
  'label-model',
  'dut_state',
  'peripherals',
  'assignee',
] as const;

export const useRepairQueueColumns = () => {
  const { mutate: claimTask, isPending: isClaimPending } = useClaimRepairTask();
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimRepairTask();
  const isPending = isClaimPending || isUnclaimPending;

  const authState = useAuthState();
  const currentUser = (authState.email || authState.identity || '').trim();

  const columns: RepairQueueColumnDef[] = useMemo(() => {
    return [
      {
        id: 'dut_id',
        header: 'Dut ID',
        accessorKey: 'dutId',
        minSize: 70,
        maxSize: 700,
        enableSorting: true,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'label-pool',
        header: 'Pool',
        accessorFn: (row) => labelValuesToString(row.pools || []),
        minSize: 70,
        maxSize: 700,
        enableSorting: true,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'label-model',
        header: 'Model',
        accessorKey: 'model',
        minSize: 70,
        maxSize: 700,
        enableSorting: true,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'dut_state',
        header: 'State',
        accessorKey: 'state',
        minSize: 70,
        maxSize: 700,
        enableSorting: true,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <DutStateCell state={cell.getValue<string>()} />
        ),
      },
      {
        id: 'peripherals',
        header: 'Peripherals (W / B / S)',
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
                  mb: 1.5,
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
              <Link
                href="http://go/fleet-console#peripherals"
                target="_blank"
                rel="noopener"
                variant="body2"
                sx={{ fontWeight: 500 }}
              >
                Learn more at go/fleet-console
              </Link>
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
        id: 'assignee',
        header: 'Assignee',
        muiTableHeadCellProps: {
          align: 'center',
        },
        muiTableBodyCellProps: {
          align: 'center',
        },
        accessorFn: (row) => row.claimedBy ?? '',
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          const claimedBy = row.original.claimedBy || '';
          const taskId = row.original.taskId;

          const trimmedClaimedBy = claimedBy.trim();

          if (trimmedClaimedBy) {
            const displayClaimedBy = trimmedClaimedBy.replace(/^user:/, '');
            const isSelf = Boolean(
              currentUser &&
                (trimmedClaimedBy === currentUser ||
                  (currentUser.startsWith('user:') &&
                    trimmedClaimedBy === currentUser.replace(/^user:/, '')) ||
                  (trimmedClaimedBy.startsWith('user:') &&
                    trimmedClaimedBy.replace(/^user:/, '') === currentUser)),
            );

            const tooltipTitle = isSelf
              ? 'Assigned to you (click to unclaim)'
              : `Assigned to ${displayClaimedBy} (click to assign to yourself)`;

            const initial = displayClaimedBy.charAt(0).toUpperCase();

            const handleClick = () => {
              if (isPending) return;
              if (isSelf) {
                unclaimTask({ taskId });
              } else {
                claimTask({ taskId });
              }
            };

            return (
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'center',
                  width: '100%',
                }}
              >
                <Tooltip title={tooltipTitle}>
                  <Avatar
                    onClick={handleClick}
                    sx={{
                      width: 26,
                      height: 26,
                      fontSize: '0.85rem',
                      bgcolor: '#0F9D58',
                      cursor: isPending ? 'not-allowed' : 'pointer',
                      opacity: isPending ? 0.6 : 1,
                      pointerEvents: isPending ? 'none' : 'auto',
                      '&:hover': {
                        opacity: isPending ? 0.6 : 0.8,
                      },
                    }}
                  >
                    {initial}
                  </Avatar>
                </Tooltip>
              </Box>
            );
          }

          return (
            <Box
              sx={{ display: 'flex', justifyContent: 'center', width: '100%' }}
            >
              <Button
                variant="outlined"
                size="small"
                disabled={isPending}
                sx={{
                  borderRadius: '16px',
                  textTransform: 'none',
                  minWidth: '64px',
                  height: '26px',
                }}
                onClick={() => claimTask({ taskId })}
              >
                Claim
              </Button>
            </Box>
          );
        },
      },
    ];
  }, [claimTask, unclaimTask, currentUser, isPending]);

  return { columns };
};
