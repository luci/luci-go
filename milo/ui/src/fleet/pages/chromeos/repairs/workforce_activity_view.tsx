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
import PersonIcon from '@mui/icons-material/Person';
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
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFeatureFlag } from '@/common/feature_flags';
import {
  displayCompactDuration,
  parseProtoDuration,
  parseProtoDurationStr,
} from '@/common/tools/time_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import {
  CHROMEOS_PLATFORM,
  generateRepairsURL,
  platformToURL,
} from '@/fleet/constants/paths';
import {
  enableChromeOsWorkforceActivity,
  enableWorkforceInProgressRepairs,
  enableWorkforceMttr,
  enableWorkforcePickupRank,
  enableWorkforcePriorityScoreCleared,
  enableWorkforceTimeframeFilter,
} from '@/fleet/features';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import {
  GetWorkforceActivityRequest,
  TechnicianActivity,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { Duration as ProtoDuration } from '@/proto/google/protobuf/duration.pb';

import { useWorkforceActivity } from './use_workforce_activity';
import { UserAvatar } from './user_avatar';

const formatDuration = (d?: ProtoDuration | string | null): string => {
  if (!d) return '-';
  if (typeof d === 'string') {
    if (d.endsWith('s') && !isNaN(Number(d.slice(0, -1)))) {
      const [str] = displayCompactDuration(parseProtoDurationStr(d));
      return str;
    }
    return d;
  }
  if (d.seconds !== undefined && !isNaN(Number(d.seconds))) {
    const [str] = displayCompactDuration(parseProtoDuration(d));
    return str;
  }
  return '-';
};

export type TimeframeOption = '1D' | '7D' | '30D' | 'YTD';

export interface WorkforceActivityViewProps {
  inProgressCount?: number;
  onBackToQueue?: () => void;
}

interface WorkforceColumn {
  id: string;
  align?: 'center';
  header: React.ReactNode;
  renderCell: (tech: TechnicianActivity) => React.ReactNode;
}

export const WorkforceActivityView = ({
  inProgressCount = 0,
  onBackToQueue,
}: WorkforceActivityViewProps) => {
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();
  const [timeframe, setTimeframe] = useState<TimeframeOption>('1D');
  const [unclaimedDuts, setUnclaimedDuts] = useState<Set<string>>(new Set());

  const showPriorityScoreCleared = useFeatureFlag(
    enableWorkforcePriorityScoreCleared,
  );
  const showPickupRank = useFeatureFlag(enableWorkforcePickupRank);
  const showMttr = useFeatureFlag(enableWorkforceMttr);
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
        ),
      );
    }
  };

  const request = useMemo(
    () =>
      GetWorkforceActivityRequest.fromPartial({
        timeframe,
      }),
    [timeframe],
  );

  const { data, isLoading, isPending, isError, error } =
    useWorkforceActivity(request);
  const loading = Boolean(isLoading || isPending);

  const technicians = useMemo(() => {
    const rawTechnicians = data?.technicians ?? [];
    return rawTechnicians.map((tech) => ({
      ...tech,
      claimedDuts: (tech.claimedDuts ?? []).filter(
        (d) => !unclaimedDuts.has(`${tech.id}:${d.dutId}`),
      ),
    }));
  }, [data?.technicians, unclaimedDuts]);

  // TODO: Properly implement this in the backend. This is just a mock.
  const handleUnclaimDut = (techId: string, dutId: string) => {
    setUnclaimedDuts((prev) => {
      const next = new Set(prev);
      next.add(`${techId}:${dutId}`);
      return next;
    });
  };

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
        header: 'Currently Claimed DUTs (In-Progress)',
        renderCell: (tech) =>
          tech.claimedDuts.length > 0 ? (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
              {tech.claimedDuts.map((dut) => (
                <Chip
                  key={dut.dutId}
                  icon={<PersonIcon sx={{ fontSize: 16 }} />}
                  label={`${dut.dutId} (Score: ${dut.score} | ${formatDuration(dut.duration)})`}
                  variant="outlined"
                  color="primary"
                  size="small"
                  onDelete={() => handleUnclaimDut(tech.id, dut.dutId)}
                  sx={{
                    fontFamily: 'monospace',
                    fontSize: '0.75rem',
                    bgcolor: colors.blue[50],
                  }}
                />
              ))}
            </Box>
          ) : (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ fontStyle: 'italic' }}
            >
              No active devices claimed
            </Typography>
          ),
      },
      showPriorityScoreCleared && {
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
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 0.5,
            }}
          >
            <Chip
              label={`#${tech.avgQueuePickupRank}`}
              size="small"
              sx={{
                bgcolor: tech.isRankSkewed
                  ? colors.yellow[700]
                  : colors.green[700],
                color: colors.white,
                fontWeight: 700,
                height: 24,
              }}
            />
            <Typography variant="caption" color="text.secondary">
              Sum: {tech.pickupRankSum} | {tech.pickupRankLabel}
            </Typography>
          </Box>
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
      showMttr && {
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
          <Button
            variant="outlined"
            startIcon={<ArrowBackIcon />}
            onClick={handleBackToQueue}
            sx={{
              textTransform: 'none',
              fontWeight: 600,
              borderRadius: '8px',
              px: 1.75,
              py: 0.75,
              whiteSpace: 'nowrap',
              '@container (max-width: 730px)': {
                padding: '4px 8px !important',
                fontSize: '0.75rem !important',
              },
            }}
          >
            Prioritized Manual Repair Queue
          </Button>

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
        {showPriorityScoreCleared && (
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
              High-impact score restored (today)
            </Typography>
          </Paper>
        )}

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
        {showMttr && (
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
        )}
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
                  sx={{ fontWeight: 700, color: 'text.secondary' }}
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
                    <TableCell key={col.id} align={col.align}>
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
