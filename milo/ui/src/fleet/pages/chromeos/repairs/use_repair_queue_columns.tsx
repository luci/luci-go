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
import { Box, Button, Divider, Link, Tooltip, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { Fragment, useMemo } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
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
import { UserAvatar } from './user_avatar';

export const formatPriorityScore = (score?: string | number): string => {
  const scoreStr = typeof score === 'number' ? String(score) : score || '0';
  if (!scoreStr || scoreStr === '0' || scoreStr === '-0') {
    return '0 pts';
  }
  if (scoreStr.startsWith('-')) {
    const absVal = scoreStr.slice(1);
    return `-${absVal} pts`;
  }
  const clean = scoreStr.startsWith('+') ? scoreStr.slice(1) : scoreStr;
  return `+${clean} pts`;
};

export const formatRuleWeight = (weight?: string | number): string => {
  const weightStr = typeof weight === 'number' ? String(weight) : weight || '0';
  if (!weightStr || weightStr === '0' || weightStr === '-0') {
    return '0 pts';
  }
  if (weightStr.startsWith('-')) {
    const absVal = weightStr.slice(1);
    return `-${absVal} pts`;
  }
  const clean = weightStr.startsWith('+') ? weightStr.slice(1) : weightStr;
  return `+${clean} pts`;
};

export type RepairQueueRow = RepairQueueItem;
export type RepairQueueColumnDef = MRT_ColumnDef<RepairQueueRow>;

export const REPAIR_QUEUE_COLUMN_IDS = [
  'rank',
  'dut_id',
  'label-pool',
  'label-model',
  'pool_model_health',
  'dut_state',
  'priority_score',
  'peripherals',
  'assignee',
] as const;

export interface UseRepairQueueColumnsOptions {
  pageIndex?: number;
  pageSize?: number;
}

/**
 * Caps how wide a rule's AIP-160 expression may grow inside the breakdown
 * tooltip. Longer expressions are truncated with an ellipsis so that a single
 * verbose rule cannot stretch the tooltip across the viewport, and so the
 * points column stays anchored on the right.
 */
const RULE_EXPRESSION_MAX_WIDTH = 300;

const PriorityScoreCell = ({
  item,
  rulesById,
}: {
  item: RepairQueueRow;
  rulesById: Map<string, PriorityRule>;
}) => {
  const scoreStr = item.priorityScore || '0';
  const formattedScore = formatPriorityScore(scoreStr);
  const matchedIds = item.matchedRuleIds || [];

  let tooltipContent: React.ReactNode;
  if (matchedIds.length === 0) {
    tooltipContent = (
      <Typography variant="body2">No matched priority rules (0 pts)</Typography>
    );
  } else {
    const cleanScore = scoreStr.replace(/^\+/, '');
    tooltipContent = (
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: '1fr auto',
          columnGap: 3,
          rowGap: 0.5,
          alignItems: 'baseline',
        }}
      >
        {matchedIds.map((id) => {
          const rule = rulesById.get(id);
          const expr = rule?.expressionAip160 ?? `Rule #${id}`;
          const weightStr = rule ? formatRuleWeight(rule.weight) : '';
          return (
            <Fragment key={id}>
              <Typography
                variant="body2"
                title={expr}
                sx={{
                  maxWidth: RULE_EXPRESSION_MAX_WIDTH,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {expr}
              </Typography>
              <Typography
                variant="body2"
                sx={{
                  textAlign: 'right',
                  whiteSpace: 'nowrap',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {weightStr}
              </Typography>
            </Fragment>
          );
        })}
        <Divider sx={{ gridColumn: '1 / -1', my: 0.5 }} />
        <Typography
          variant="body2"
          sx={{
            gridColumn: 2,
            textAlign: 'right',
            whiteSpace: 'nowrap',
            fontWeight: 600,
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {`= ${cleanScore} pts`}
        </Typography>
      </Box>
    );
  }

  return (
    <Tooltip
      title={tooltipContent}
      enterDelay={100}
      placement="bottom-start"
      slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
    >
      <Typography
        component="span"
        sx={{
          fontWeight: 600,
          cursor: 'help',
          display: 'inline-block',
          textDecoration: 'underline dotted',
          textUnderlineOffset: '3px',
        }}
      >
        {formattedScore}
      </Typography>
    </Tooltip>
  );
};

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
        size: 60,
        minSize: 50,
        maxSize: 80,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          return String(pageIndex * pageSize + row.index + 1);
        },
      },
      {
        id: 'dut_id',
        header: 'Dut ID',
        accessorKey: 'dutId',
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
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <EllipsisTooltip>{cell.getValue<string>()}</EllipsisTooltip>
        ),
      },
      {
        id: 'pool_model_health',
        header: 'Pool / Model Health',
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
        size: 60,
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
        id: 'dut_state',
        header: 'State',
        accessorKey: 'state',
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: ({ cell }: FC_CellProps<RepairQueueRow>) => (
          <DutStateCell state={cell.getValue<string>()} />
        ),
      },
      {
        id: 'priority_score',
        header: 'Priority Score',
        size: 140,
        minSize: 120,
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
      {
        id: 'peripherals',
        header: 'Peripherals (W / B / S)',
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
        enableSorting: false,
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
                  <UserAvatar
                    email={displayClaimedBy}
                    onClick={handleClick}
                    sx={{
                      width: 26,
                      height: 26,
                      fontSize: '0.85rem',
                      cursor: isPending ? 'not-allowed' : 'pointer',
                      opacity: isPending ? 0.6 : 1,
                      pointerEvents: isPending ? 'none' : 'auto',
                      '&:hover': {
                        opacity: isPending ? 0.6 : 0.8,
                      },
                    }}
                  />
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
