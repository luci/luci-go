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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import { ReactNode, useState } from 'react';

import { colors } from '@/fleet/theme/colors';

import { UserAvatar } from './user_avatar';
import {
  DEFAULT_WORKFORCE_LABELS,
  formatWorkforceChipDuration,
  formatWorkforceDuration,
  TimeframeOption,
  WorkforceActivityLabels,
  WorkforceClaimedDevice,
  WorkforceTechnician,
} from './workforce_helpers';

const MAX_VISIBLE_DEVICES = 2;
const CLAIMED_DEVICES_MAX_WIDTH = 480;

interface WorkforceColumn {
  id: string;
  align?: 'center';
  maxWidth?: number;
  header: ReactNode;
  renderCell: (tech: WorkforceTechnician) => ReactNode;
}

export interface WorkforceTechniciansTableProps {
  technicians: readonly WorkforceTechnician[];
  timeframe: TimeframeOption;
  loading: boolean;
  isError: boolean;
  showPickupRank: boolean;
  showInProgressRepairs: boolean;
  onUnclaimTask?: (taskId: string) => void;
  isUnclaimPending?: boolean;
  labels?: WorkforceActivityLabels;
}

export const WorkforceTechniciansTable = ({
  technicians,
  timeframe,
  loading,
  isError,
  showPickupRank,
  showInProgressRepairs,
  onUnclaimTask,
  isUnclaimPending = false,
  labels = DEFAULT_WORKFORCE_LABELS,
}: WorkforceTechniciansTableProps) => {
  const columns: WorkforceColumn[] = [
    {
      id: 'technician',
      header: 'Technician',
      renderCell: (tech) => (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <UserAvatar
            name={tech.name}
            email={tech.email}
            id={tech.id}
            sx={{ width: 36, height: 36, fontSize: '0.95rem' }}
          />
          <Box>
            <Typography variant="body2" sx={{ fontWeight: 700 }}>
              {tech.name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {tech.email}
            </Typography>
          </Box>
        </Box>
      ),
    },
    ...(showInProgressRepairs
      ? [
          {
            id: 'inProgressRepairs',
            maxWidth: CLAIMED_DEVICES_MAX_WIDTH,
            header: labels.claimedColumnHeader,
            renderCell: (tech: WorkforceTechnician) => (
              <ClaimedDevicesCell
                claimedDevices={tech.claimedDevices}
                emptyText={labels.emptyClaimedText}
                onUnclaimTask={onUnclaimTask}
                isUnclaimPending={isUnclaimPending}
              />
            ),
          },
        ]
      : []),
    {
      id: 'priorityScoreCleared',
      align: 'center',
      header: `Priority Score Cleared (${timeframe})`,
      renderCell: (tech) => (
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 0.5,
          }}
        >
          <Chip
            label={`${tech.priorityScoreCleared} pts`}
            size="small"
            sx={{
              bgcolor: 'primary.main',
              color: 'primary.contrastText',
              fontWeight: 700,
              height: 24,
            }}
          />
          <Typography variant="caption" color="text.secondary">
            avg {tech.avgPtsPerDevice} pts/{labels.scoreUnit}
          </Typography>
        </Box>
      ),
    },
    ...(showPickupRank
      ? [
          {
            id: 'pickupRank',
            align: 'center' as const,
            header: (
              <Box
                sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}
              >
                <span>Avg Queue Pickup Rank ({timeframe})</span>
                <Tooltip title="Average position of tasks in the priority queue when claimed by the technician.">
                  <Box
                    component="span"
                    tabIndex={0}
                    sx={{ display: 'inline-flex', cursor: 'help' }}
                  >
                    <HelpOutlineIcon
                      sx={{ fontSize: 16, color: 'text.secondary' }}
                    />
                  </Box>
                </Tooltip>
              </Box>
            ),
            renderCell: (tech: WorkforceTechnician) => (
              <Chip
                label={`#${tech.avgQueuePickupRank}`}
                sx={{
                  bgcolor: colors.green[700],
                  color: colors.white,
                  fontWeight: 700,
                  height: 28,
                  fontSize: '0.875rem',
                }}
              />
            ),
          },
        ]
      : []),
    {
      id: 'completedRepairs',
      align: 'center',
      header: `Completed Repairs (${timeframe})`,
      renderCell: (tech) => (
        <Typography variant="body2" sx={{ fontWeight: 700 }}>
          {tech.completedRepairs} {labels.completedUnit}
        </Typography>
      ),
    },
    {
      id: 'mttr',
      align: 'center',
      header: 'Avg Duration (MTTR)',
      renderCell: (tech) => (
        <Typography variant="body2" color="text.secondary">
          {formatWorkforceDuration(tech.avgDuration)}
        </Typography>
      ),
    },
  ];

  const renderStatusRow = (testId: string, content: ReactNode, py = 4) => (
    <TableRow>
      <TableCell
        colSpan={columns.length}
        align="center"
        sx={{ color: 'text.secondary', py }}
        data-testid={testId}
      >
        {content}
      </TableCell>
    </TableRow>
  );

  const renderTableBody = (): ReactNode => {
    if (loading) {
      return renderStatusRow(
        'workforce-loading-spinner',
        <CircularProgress size={36} />,
        6,
      );
    }
    if (isError) {
      return renderStatusRow(
        'workforce-error-table',
        'Failed to load workforce activity data.',
      );
    }
    if (technicians.length === 0) {
      return renderStatusRow(
        'workforce-empty-table',
        `No workforce activity recorded for the selected timeframe (${timeframe}).`,
      );
    }
    return technicians.map((tech) => (
      <TableRow key={tech.id} hover>
        {columns.map((col) => (
          <TableCell
            key={col.id}
            align={col.align}
            sx={col.maxWidth ? { maxWidth: col.maxWidth } : undefined}
          >
            {col.renderCell(tech)}
          </TableCell>
        ))}
      </TableRow>
    ));
  };

  return (
    <TableContainer
      component={Paper}
      elevation={0}
      sx={{ border: `1px solid ${colors.grey[300]}`, borderRadius: '8px' }}
    >
      <Table>
        <TableHead sx={{ bgcolor: colors.grey[50] }}>
          <TableRow>
            {columns.map((col) => (
              <TableCell
                key={col.id}
                align={col.align}
                sx={{
                  fontWeight: 700,
                  color: 'text.secondary',
                  ...(col.maxWidth ? { maxWidth: col.maxWidth } : {}),
                }}
              >
                {col.header}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>{renderTableBody()}</TableBody>
      </Table>
    </TableContainer>
  );
};

interface ClaimedDevicesCellProps {
  claimedDevices: readonly WorkforceClaimedDevice[];
  emptyText: string;
  onUnclaimTask?: (taskId: string) => void;
  isUnclaimPending: boolean;
}

const ClaimedDevicesCell = ({
  claimedDevices,
  emptyText,
  onUnclaimTask,
  isUnclaimPending,
}: ClaimedDevicesCellProps) => {
  const [expanded, setExpanded] = useState(false);

  if (claimedDevices.length === 0) {
    return (
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ fontStyle: 'italic' }}
      >
        {emptyText}
      </Typography>
    );
  }

  const visibleDevices = expanded
    ? claimedDevices
    : claimedDevices.slice(0, MAX_VISIBLE_DEVICES);
  const hiddenCount = claimedDevices.length - visibleDevices.length;

  return (
    <Box
      sx={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: 1,
        maxWidth: CLAIMED_DEVICES_MAX_WIDTH,
      }}
    >
      {visibleDevices.map((device) => {
        const hasTaskId = Boolean(device.taskId && device.taskId !== '0');
        return (
          <Chip
            key={hasTaskId ? device.taskId : device.deviceId}
            label={`${device.deviceId} (Score: ${device.score} | ${formatWorkforceChipDuration(device.duration)})`}
            variant="filter"
            size="small"
            disabled={isUnclaimPending}
            onDelete={
              hasTaskId && onUnclaimTask
                ? () => onUnclaimTask(device.taskId)
                : undefined
            }
            sx={{
              fontFamily: 'monospace',
              fontSize: '0.75rem',
              height: 'auto',
              minHeight: 24,
              py: 0.25,
              '& .MuiChip-label': {
                display: 'block',
                whiteSpace: 'normal',
              },
            }}
          />
        );
      })}
      {claimedDevices.length > MAX_VISIBLE_DEVICES && (
        <Button
          variant="text"
          size="small"
          onClick={() => setExpanded((prev) => !prev)}
          sx={{
            textTransform: 'none',
            fontSize: '0.75rem',
            fontWeight: 600,
            py: 0,
            px: 1,
            minWidth: 'auto',
            height: 24,
          }}
        >
          {expanded ? 'Show less' : `And ${hiddenCount} more...`}
        </Button>
      )}
    </Box>
  );
};
