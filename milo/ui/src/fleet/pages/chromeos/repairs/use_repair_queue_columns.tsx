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

import BugReportOutlinedIcon from '@mui/icons-material/BugReportOutlined';
import CircleIcon from '@mui/icons-material/Circle';
import { Box, Chip, Divider, Tooltip, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
import { AssigneeCell } from '@/fleet/components/repair_queue/assignee_cell';
import { PriorityScoreCell } from '@/fleet/components/repair_queue/priority_score_cell';
import {
  getBugUrl,
  parsePriorityRules,
} from '@/fleet/components/repair_queue/utils';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { DutStateCell } from '@/fleet/pages/device_list_page/chromeos/dut_state_cell';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';
import { getErrorMessage } from '@/fleet/utils/errors';
import { RepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { PeripheralsCell } from './peripheral_state_indicator';
import {
  useClaimRepairTask,
  useUnclaimRepairTask,
} from './use_claim_repair_task';
import { useIrmIncidents } from './use_irm_incidents';
import { usePriorityRules } from './use_priority_rules';
import { useRepairQueueFilterBuilders } from './use_repair_queue_filter_builders';

export type RepairQueueRow = RepairQueueItem;
export type RepairQueueColumnDef = MRT_ColumnDef<RepairQueueRow>;

export const REPAIR_QUEUE_COLUMN_IDS = [
  'rank',
  'id',
  'label-pool',
  'label-model',
  'dut_state',
  'active_irm',
  'assignee',
  'peripherals',
  'pool_model_health',
  'priority_score',
] as const;

export interface UseRepairQueueColumnsOptions {
  pageIndex?: number;
  pageSize?: number;
  onError?: (message: string) => void;
}

const BaseDeviceIdCell = renderCellWithLink<RepairQueueRow>({
  linkGenerator: (value) => {
    const trimmed = (value ?? '').trim();
    if (!trimmed) {
      return '';
    }
    return generateChromeOsDeviceDetailsURL(trimmed);
  },
  newTab: true,
});

const DeviceIdCell = (props: FC_CellProps<RepairQueueRow>) => {
  const value = String(props.cell.getValue() ?? '').trim();
  if (!value) {
    return <EllipsisTooltip>-</EllipsisTooltip>;
  }
  return <BaseDeviceIdCell {...props} />;
};

const renderHealthMetric = (
  label: string,
  actualPct?: number,
  inRepairPct?: number,
) => {
  if (actualPct === undefined || actualPct === null) {
    return <span>-</span>;
  }
  const actualStr = `${Math.round(actualPct * 100)}%`;
  const inRepairRound =
    inRepairPct !== undefined && inRepairPct !== null
      ? Math.round(inRepairPct * 100)
      : 0;

  if (inRepairRound <= 0) {
    return <span>{actualStr}</span>;
  }

  const effectiveRound = Math.round(actualPct * 100) + inRepairRound;

  const tooltipContent = (
    <Box>
      <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 0.5 }}>
        {label} Health Breakdown
      </Typography>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'auto auto',
          columnGap: 2,
          rowGap: 0.25,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          Actual available:
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 500, textAlign: 'right' }}
        >
          {actualStr}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          In repair (claimed):
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 500, textAlign: 'right' }}
        >
          +{inRepairRound}%
        </Typography>
        <Divider sx={{ gridColumn: '1 / -1', my: 0.5 }} />
        <Typography variant="body2" color="text.secondary">
          Effective health:
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontWeight: 600, textAlign: 'right' }}
        >
          {effectiveRound}%
        </Typography>
      </Box>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ mt: 0.75, display: 'block', fontStyle: 'italic' }}
      >
        * Effective health counts claimed devices as healthy for priority rules.
      </Typography>
    </Box>
  );

  return (
    <span>
      {actualStr}{' '}
      <Tooltip
        title={tooltipContent}
        arrow
        enterDelay={100}
        placement="bottom-start"
        slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
      >
        <span
          data-testid={`${label.toLowerCase()}-in-repair-badge`}
          style={{
            borderBottom: '1px dashed currentColor',
            cursor: 'help',
            fontWeight: 500,
          }}
        >
          (+{inRepairRound}%)
        </span>
      </Tooltip>
    </span>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useRepairQueueColumns = ({
  pageIndex = 0,
  pageSize = 100,
  onError,
}: UseRepairQueueColumnsOptions = {}) => {
  const { mutate: claimTask, isPending: isClaimPending } = useClaimRepairTask({
    onError: (err) => {
      onError?.(getErrorMessage(err, 'claim repair task'));
    },
  });
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimRepairTask({
      onError: (err) => {
        onError?.(getErrorMessage(err, 'unclaim repair task'));
      },
    });
  const isPending = isClaimPending || isUnclaimPending;

  const authState = useAuthState();
  const currentUser = (authState.email || authState.identity || '').trim();

  const { filterBuilders } = useRepairQueueFilterBuilders();

  const { rules } = usePriorityRules();
  const rulesById = useMemo(
    () => parsePriorityRules(rules, filterBuilders),
    [rules, filterBuilders],
  );

  const { data: irmData } = useIrmIncidents();
  const irmTitleByBugId = useMemo(() => {
    const map = new Map<string, string>();
    for (const inc of irmData?.irmIncidents ?? []) {
      const bugId = inc.masterBugId?.trim();
      if (bugId) {
        map.set(bugId, inc.title?.trim() || `Incident ${bugId}`);
      }
    }
    return map;
  }, [irmData]);

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
        id: 'id',
        header: 'ID',
        accessorFn: (row) => row.deviceId || '',
        size: 100,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        Cell: DeviceIdCell,
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
        id: 'active_irm',
        header: 'Active IRM',
        size: 110,
        minSize: 80,
        maxSize: 400,
        enableSorting: false,
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          const activeIrmIds = Array.from(
            new Set(
              (row.original.activeIrmIds || [])
                .map((id) => id.trim())
                .filter(Boolean),
            ),
          );
          if (activeIrmIds.length === 0) {
            return (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontSize: '13px' }}
              >
                -
              </Typography>
            );
          }
          return (
            <Box
              sx={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 0.5,
                alignItems: 'center',
              }}
            >
              {activeIrmIds.map((bugId) => {
                const title = irmTitleByBugId.get(bugId) ?? `b/${bugId}`;
                return (
                  <Tooltip key={bugId} title={title} enterDelay={100}>
                    <Chip
                      component="a"
                      href={getBugUrl(bugId)}
                      target="_blank"
                      rel="noopener noreferrer"
                      onClick={(e) => e.stopPropagation()}
                      icon={<BugReportOutlinedIcon />}
                      label={`b/${bugId}`}
                      clickable
                      size="small"
                      color="error"
                      variant="outlined"
                      sx={{ fontWeight: 600 }}
                    />
                  </Tooltip>
                );
              })}
            </Box>
          );
        },
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
            etag={row.original.etag}
            currentUser={currentUser}
            isPending={isPending}
            claimTask={claimTask}
            unclaimTask={unclaimTask}
          />
        ),
      },
      {
        id: 'peripherals',
        header: 'Peripherals',
        size: 110,
        minSize: 70,
        maxSize: 700,
        enableSorting: false,
        meta: {
          infoTooltip: (
            <Box sx={{ maxWidth: 360 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
                Peripheral Health States
              </Typography>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ mb: 1.5 }}
              >
                Displays configured peripherals (Wi-Fi, Bluetooth, Servo,
                Chameleon, Audiobox) for each DUT:
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 1,
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <CircleIcon sx={{ color: colors.green[500], fontSize: 14 }} />
                  <Typography variant="body2">
                    <strong>Green</strong> &mdash; Healthy &amp; responsive (OK)
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <CircleIcon sx={{ color: colors.red[600], fontSize: 14 }} />
                  <Typography variant="body2">
                    <strong>Red</strong> &mdash; Hardware failure (BROKEN)
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <CircleIcon
                    sx={{ color: colors.orange[600], fontSize: 14 }}
                  />
                  <Typography variant="body2">
                    <strong>Orange</strong> &mdash; Disconnected / unplugged
                    (MISSING)
                  </Typography>
                </Box>
              </Box>
            </Box>
          ),
        },
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => (
          <PeripheralsCell
            wifiState={row.original.wifiState}
            bluetoothState={row.original.bluetoothState}
            servoState={row.original.servoState}
            chameleonState={row.original.chameleonState}
            audioboxJackpluggerState={row.original.audioboxJackpluggerState}
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
        meta: {
          infoTooltip:
            'Percentage of devices in the assigned pool(s) and model(s) that ' +
            'are healthy (not in manual repair or repair failed). When devices ' +
            'in that pool or model are currently claimed in repair, in-flight ' +
            'repairs (+X%) are indicated with a dashed underline and count ' +
            'toward priority scoring rules.',
        },
        Cell: ({ row }: FC_CellProps<RepairQueueRow>) => {
          const poolHealth = row.original.poolHealthPct;
          const poolInRepair = row.original.poolInRepairPct;
          const modelHealth = row.original.modelHealthPct;
          const modelInRepair = row.original.modelInRepairPct;

          const poolInRepairRound =
            poolInRepair !== undefined && poolInRepair !== null
              ? Math.round(poolInRepair * 100)
              : 0;
          const modelInRepairRound =
            modelInRepair !== undefined && modelInRepair !== null
              ? Math.round(modelInRepair * 100)
              : 0;

          if (poolInRepairRound <= 0 && modelInRepairRound <= 0) {
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
          }

          return (
            <Typography
              variant="body2"
              sx={{
                fontSize: '13px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: 0.5,
                whiteSpace: 'nowrap',
              }}
            >
              {renderHealthMetric('Pool', poolHealth, poolInRepair)}
              <span>/</span>
              {renderHealthMetric('Model', modelHealth, modelInRepair)}
            </Typography>
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
    irmTitleByBugId,
  ]);

  return { columns };
};
