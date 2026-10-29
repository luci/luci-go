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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import FormatListBulletedIcon from '@mui/icons-material/FormatListBulleted';
import GroupIcon from '@mui/icons-material/Group';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import HowToRegIcon from '@mui/icons-material/HowToReg';
import TimerIcon from '@mui/icons-material/Timer';
import TrendingUpIcon from '@mui/icons-material/TrendingUp';
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import { Duration } from 'luxon';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { UserAvatar } from '@/fleet/components/repair_queue/user_avatar';
import {
  CHROMEOS_PLATFORM,
  generateRepairsURL,
  platformToURL,
} from '@/fleet/constants/paths';
import {
  enableChromeOsWorkforceActivity,
  enableWorkforceInProgressRepairs,
  enableWorkforcePickupRank,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import { colors } from '@/fleet/theme/colors';
import { prettySeconds } from '@/fleet/utils/dates';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  GetWorkforceActivityRequest,
  GetWorkforceActivityRequest_Timeframe,
  TechnicianActivity,
  UnclaimRepairTaskRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { Duration as ProtoDuration } from '@/proto/google/protobuf/duration.pb';

import { LabSelector } from './lab_selector';
import { useUnclaimRepairTask } from './use_claim_repair_task';
import { RepairLab, useRepairLab } from './use_repair_lab';
import { useWorkforceActivity } from './use_workforce_activity';

const MAX_VISIBLE_DUTS = 2;
const CLAIMED_DUTS_MAX_WIDTH = 480;

const formatDuration = (d?: ProtoDuration): string =>
  d ? prettySeconds(Number(d.seconds)) : '-';

const formatChipDuration = (d?: ProtoDuration): string => {
  if (!d) return '-';
  if (!Number(d.seconds)) return '0s';

  return Duration.fromObject({ seconds: Number(d.seconds) })
    .rescale()
    .toHuman({ unitDisplay: 'short' })
    .split(', ')
    .slice(0, 2)
    .join(', ');
};

export type TimeframeOption = '1D' | '7D' | '30D' | 'YTD';

const TIMEFRAME_TO_PROTO: Record<
  TimeframeOption,
  GetWorkforceActivityRequest_Timeframe
> = {
  '1D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_1D,
  '7D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_7D,
  '30D': GetWorkforceActivityRequest_Timeframe.TIMEFRAME_30D,
  YTD: GetWorkforceActivityRequest_Timeframe.TIMEFRAME_YTD,
};
export interface WorkforceActivityViewProps {
  inProgressCount?: number;
  onBackToQueue?: () => void;
  lab?: RepairLab;
}

interface WorkforceColumn {
  id: string;
  align?: 'center';
  maxWidth?: number;
  header: React.ReactNode;
  renderCell: (tech: TechnicianActivity) => React.ReactNode;
}

export const WorkforceActivityView = ({
  inProgressCount = 0,
  onBackToQueue,
  lab: labProp,
}: WorkforceActivityViewProps) => {
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();
  const { lab: contextLab } = useRepairLab();
  const lab = labProp ?? contextLab;
  const [timeframe, setTimeframe] = useState<TimeframeOption>('1D');
  const [expandedTechIds, setExpandedTechIds] = useState<
    Record<string, boolean>
  >({});
  const { mutate: unclaimTask, isPending: isUnclaimPending } =
    useUnclaimRepairTask();

  const showPickupRank = useFeatureFlag(enableWorkforcePickupRank);
  const showInProgressRepairs = useFeatureFlag(
    enableWorkforceInProgressRepairs,
  );
  const showTimeframeFilter = useFeatureFlag(enableWorkforceTimeframeFilter);

  const handleBackToQueue = () => {
    if (onBackToQueue) {
      onBackToQueue();
    } else {
      navigate(
        generateRepairsURL(
          currentPlatform ? platformToURL(currentPlatform) : CHROMEOS_PLATFORM,
        ) + `?lab=${lab}`,
      );
    }
  };

  const request = useMemo(
    () =>
      GetWorkforceActivityRequest.fromPartial({
        timeframe: TIMEFRAME_TO_PROTO[timeframe],
        lab,
      }),
    [timeframe, lab],
  );

  const { data, isLoading, isPending, isError, error } =
    useWorkforceActivity(request);
  const loading = Boolean(isLoading || isPending);

  const technicians = data?.technicians ?? [];

  const getColumns = (): WorkforceColumn[] => {
    const allColumns: (WorkforceColumn | false)[] = [
      {
        id: 'technician',
        header: 'Technician',
        renderCell: (tech) => (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <UserAvatar
              name={tech.name}
              email={tech.email}
              id={tech.id}
              sx={{
                width: 36,
                height: 36,
                fontSize: '0.95rem',
              }}
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
      showInProgressRepairs && {
        id: 'inProgressRepairs',
        maxWidth: CLAIMED_DUTS_MAX_WIDTH,
        header: 'Currently Claimed DUTs (In-Progress)',
        renderCell: (tech) => {
          const isExpanded = Boolean(expandedTechIds[tech.id]);
          const visibleDuts = isExpanded
            ? tech.claimedDuts
            : tech.claimedDuts.slice(0, MAX_VISIBLE_DUTS);
          const hiddenCount = tech.claimedDuts.length - visibleDuts.length;

          return tech.claimedDuts.length > 0 ? (
            <Box
              sx={{
                display: 'flex',
                flexWrap: 'wrap',
                alignItems: 'center',
                gap: 1,
                maxWidth: CLAIMED_DUTS_MAX_WIDTH,
              }}
            >
              {visibleDuts.map((dut) => (
                <Chip
                  key={dut.dutId}
                  label={`${dut.dutId} (Score: ${dut.score} | ${formatChipDuration(dut.duration)})`}
                  variant="outlined"
                  color="primary"
                  size="small"
                  disabled={isUnclaimPending}
                  onDelete={
                    dut.taskId && dut.taskId !== '0'
                      ? () =>
                          unclaimTask(
                            UnclaimRepairTaskRequest.fromPartial({
                              taskId: dut.taskId,
                            }),
                          )
                      : undefined
                  }
                  sx={{
                    fontFamily: 'monospace',
                    fontSize: '0.75rem',
                    bgcolor: colors.blue[50],
                    height: 'auto',
                    minHeight: 24,
                    py: 0.25,
                    '& .MuiChip-label': {
                      display: 'block',
                      whiteSpace: 'normal',
                    },
                  }}
                />
              ))}
              {tech.claimedDuts.length > MAX_VISIBLE_DUTS && (
                <Button
                  variant="text"
                  size="small"
                  onClick={() =>
                    setExpandedTechIds((prev) => ({
                      ...prev,
                      [tech.id]: !isExpanded,
                    }))
                  }
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
                  {isExpanded ? 'Show less' : `And ${hiddenCount} more...`}
                </Button>
              )}
            </Box>
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontStyle: 'italic' }}
            >
              No active devices claimed
            </Typography>
          );
        },
      },
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
              avg {tech.avgPtsPerDut} pts/DUT
            </Typography>
          </Box>
        ),
      },
      showPickupRank && {
        id: 'pickupRank',
        align: 'center',
        header: (
          <Box
            sx={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 0.5,
            }}
          >
            <span>Avg Queue Pickup Rank ({timeframe})</span>
            <Tooltip title="Average position of tasks in the priority queue when claimed by the technician.">
              <HelpOutlineIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
            </Tooltip>
          </Box>
        ),
        renderCell: (tech) => (
          <Chip
            label={`#${tech.avgQueuePickupRank}`}
            sx={{
              bgcolor: tech.isRankSkewed
                ? colors.yellow[700]
                : colors.green[700],
              color: colors.white,
              fontWeight: 700,
              height: 28,
              fontSize: '0.875rem',
            }}
          />
        ),
      },
      {
        id: 'completedRepairs',
        align: 'center',
        header: `Completed Repairs (${timeframe})`,
        renderCell: (tech) => (
          <Typography variant="body2" sx={{ fontWeight: 700 }}>
            {tech.completedRepairs} devices
          </Typography>
        ),
      },
      {
        id: 'mttr',
        align: 'center',
        header: 'Avg Duration (MTTR)',
        renderCell: (tech) => (
          <Typography variant="body2" color="text.secondary">
            {formatDuration(tech.avgDuration)}
          </Typography>
        ),
      },
    ];

    return allColumns.filter((col): col is WorkforceColumn => Boolean(col));
  };

  const columns = getColumns();

  return (
    <Box sx={{ containerType: 'inline-size' }}>
      {/* Header Row */}
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'nowrap',
          gap: 2,
          mb: 2.5,
          '@container (max-width: 900px)': {
            flexDirection: 'column',
            alignItems: 'stretch !important',
          },
        }}
      >
        <Box sx={{ flex: '1 1 0%', minWidth: 0 }}>
          <Typography variant="h4" sx={{ fontWeight: 700 }}>
            Workforce &amp; Technician Monitoring
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Real-time technician throughput tracking, active repair assignments,
            and average performance metrics.
          </Typography>
        </Box>

        <Box
          sx={{
            flex: '0 0 auto',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'flex-end',
            gap: 1.5,
            '@container (max-width: 900px)': {
              width: '100%',
              flexDirection: 'row !important',
              flexWrap: 'wrap',
              justifyContent: 'space-between',
              alignItems: 'center !important',
              gap: '12px !important',
            },
            '@container (max-width: 730px)': {
              gap: '8px !important',
            },
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 2,
              flexWrap: 'wrap',
            }}
          >
            <LabSelector value={lab} />
            <Button
              variant="toolbar"
              startIcon={<ArrowBackIcon />}
              onClick={handleBackToQueue}
            >
              Prioritized Manual Repair Queue
            </Button>
          </Box>

          {showTimeframeFilter && (
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                flexWrap: 'wrap',
                gap: 1,
                '@container (max-width: 730px)': {
                  gap: '6px !important',
                },
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{
                  fontWeight: 600,
                  '@container (max-width: 730px)': {
                    fontSize: '0.75rem !important',
                  },
                }}
              >
                Timeframe:
              </Typography>
              <ToggleButtonGroup
                value={timeframe}
                exclusive
                size="small"
                onChange={(_, newValue: TimeframeOption | null) => {
                  if (newValue) setTimeframe(newValue);
                }}
                sx={{
                  '& .MuiToggleButton-root': {
                    textTransform: 'none',
                    px: 1.25,
                    py: 0.5,
                    fontSize: '0.8125rem',
                    whiteSpace: 'nowrap',
                  },
                  '@container (max-width: 730px)': {
                    '& .MuiToggleButton-root': {
                      padding: '3px 6px !important',
                      fontSize: '0.75rem !important',
                    },
                  },
                }}
              >
                <ToggleButton value="1D">Last Day (1D)</ToggleButton>
                <ToggleButton value="7D">Last Week (7D)</ToggleButton>
                <ToggleButton value="30D">Last Month (30D)</ToggleButton>
                <ToggleButton value="YTD">YTD</ToggleButton>
              </ToggleButtonGroup>
            </Box>
          )}
        </Box>
      </Box>

      {/* Error Alert */}
      {isError && (
        <Alert
          severity="error"
          sx={{ mb: 2.5 }}
          data-testid="workforce-activity-error"
        >
          {getErrorMessage(error, 'loading workforce activity data')}
        </Alert>
      )}

      {/* KPI Summary Cards */}
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))',
          gap: 2,
          mb: 3,
        }}
      >
        {/* Active Repairers */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: '8px',
            border: `1px solid ${colors.grey[300]}`,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              mb: 1,
            }}
          >
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontWeight: 600 }}
            >
              Active Repairers
            </Typography>
            <GroupIcon sx={{ color: 'primary.main', fontSize: 20 }} />
          </Box>
          {loading ? (
            <Skeleton
              variant="text"
              width={60}
              height={42}
              sx={{ mb: 0.5 }}
              data-testid="kpi-skeleton"
            />
          ) : (
            <Typography variant="h4" sx={{ fontWeight: 700, mb: 0.5 }}>
              {data?.activeRepairers ?? 0}
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            Contributing in selected window ({timeframe})
          </Typography>
        </Paper>

        {/* Total Priority Points Cleared */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: '8px',
            border: `1px solid ${colors.grey[300]}`,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              mb: 1,
            }}
          >
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontWeight: 600 }}
            >
              Total Priority Points Cleared
            </Typography>
            <TrendingUpIcon sx={{ color: 'primary.main', fontSize: 20 }} />
          </Box>
          {loading ? (
            <Skeleton
              variant="text"
              width={100}
              height={42}
              sx={{ mb: 0.5 }}
              data-testid="kpi-skeleton"
            />
          ) : (
            <Typography
              variant="h4"
              sx={{ fontWeight: 700, color: 'primary.main', mb: 0.5 }}
            >
              {(data?.totalPriorityPointsCleared ?? 0).toLocaleString()} pts
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            High-impact score restored ({timeframe})
          </Typography>
        </Paper>

        {/* Avg Queue Pickup Rank */}
        {showPickupRank && (
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: '8px',
              border: `1px solid ${colors.grey[300]}`,
            }}
          >
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                mb: 1,
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontWeight: 600 }}
              >
                Avg Queue Pickup Rank
              </Typography>
              <FormatListBulletedIcon
                sx={{ color: 'primary.main', fontSize: 20 }}
              />
            </Box>
            {loading ? (
              <Skeleton
                variant="text"
                width={70}
                height={42}
                sx={{ mb: 0.5 }}
                data-testid="kpi-skeleton"
              />
            ) : (
              <Typography
                variant="h4"
                sx={{ fontWeight: 700, color: 'primary.main', mb: 0.5 }}
              >
                #{data?.avgQueuePickupRank ?? 0}
              </Typography>
            )}
            <Typography variant="caption" color="text.secondary">
              Mean pickup position (Anti-skew)
            </Typography>
          </Paper>
        )}

        {/* In-Progress Repairs */}
        {showInProgressRepairs && (
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: '8px',
              border: `1px solid ${colors.grey[300]}`,
            }}
          >
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                mb: 1,
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontWeight: 600 }}
              >
                In-Progress Repairs
              </Typography>
              <HowToRegIcon sx={{ color: colors.orange[600], fontSize: 20 }} />
            </Box>
            {loading ? (
              <Skeleton
                variant="text"
                width={60}
                height={42}
                sx={{ mb: 0.5 }}
                data-testid="kpi-skeleton"
              />
            ) : (
              <Typography
                variant="h4"
                sx={{ fontWeight: 700, color: colors.orange[600], mb: 0.5 }}
              >
                {data?.inProgressRepairs ?? inProgressCount}
              </Typography>
            )}
            <Typography variant="caption" color="text.secondary">
              Currently claimed physical DUTs
            </Typography>
          </Paper>
        )}

        {/* Avg Repair Time (MTTR) */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: '8px',
            border: `1px solid ${colors.grey[300]}`,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              mb: 1,
            }}
          >
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontWeight: 600 }}
            >
              Avg Repair Time (MTTR)
            </Typography>
            <TimerIcon sx={{ color: 'primary.main', fontSize: 20 }} />
          </Box>
          {loading ? (
            <Skeleton
              variant="text"
              width={90}
              height={42}
              sx={{ mb: 0.5 }}
              data-testid="kpi-skeleton"
            />
          ) : (
            <Typography
              variant="h4"
              sx={{ fontWeight: 700, color: 'primary.main', mb: 0.5 }}
            >
              {formatDuration(data?.avgRepairTime)}
            </Typography>
          )}
          <Typography variant="caption" color="text.secondary">
            Mean elapsed time ({timeframe})
          </Typography>
        </Paper>
      </Box>

      {/* Technicians Table */}
      <TableContainer
        component={Paper}
        elevation={0}
        sx={{
          border: `1px solid ${colors.grey[300]}`,
          borderRadius: '8px',
        }}
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
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  align="center"
                  sx={{ py: 6 }}
                  data-testid="workforce-loading-spinner"
                >
                  <CircularProgress size={36} />
                </TableCell>
              </TableRow>
            ) : isError ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  align="center"
                  sx={{ color: 'text.secondary', py: 4 }}
                  data-testid="workforce-error-table"
                >
                  Failed to load workforce activity data.
                </TableCell>
              </TableRow>
            ) : technicians.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  align="center"
                  sx={{ color: 'text.secondary', py: 4 }}
                  data-testid="workforce-empty-table"
                >
                  No workforce activity recorded for the selected timeframe (
                  {timeframe}).
                </TableCell>
              </TableRow>
            ) : (
              technicians.map((tech) => (
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
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export function Component() {
  const isEnabled = useFeatureFlag(enableChromeOsWorkforceActivity);

  if (!isEnabled) {
    return <PageNotFoundPage />;
  }

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-chromeos-repairs-workforce">
      <FleetHelmet pageTitle="ChromeOS Workforce Activity" />
      <RecoverableErrorBoundary key="fleet-chromeos-repairs-workforce">
        <LoggedInBoundary>
          <div
            css={{
              margin: '24px',
              paddingBottom: '40px',
            }}
          >
            <WorkforceActivityView />
          </div>
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}

export default Component;
