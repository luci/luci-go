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
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
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
import { enableChromeOsWorkforceActivity } from '@/fleet/features';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { PageNotFoundPage } from '@/fleet/pages/not_found_page';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { GetWorkforceActivityRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
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

export const WorkforceActivityView = ({
  inProgressCount = 0,
  onBackToQueue,
}: WorkforceActivityViewProps) => {
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();
  const [timeframe, setTimeframe] = useState<TimeframeOption>('1D');
  const [unclaimedDuts, setUnclaimedDuts] = useState<Set<string>>(new Set());

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

      {/* Info Banner */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
          px: 2,
          py: 1.5,
          mb: 3,
          borderRadius: '8px',
          bgcolor: colors.blue[50],
          border: `1px solid ${colors.blue[200]}`,
          color: colors.blue[900],
        }}
      >
        <InfoOutlinedIcon sx={{ color: 'primary.main', fontSize: 22 }} />
        <Typography variant="body2" sx={{ fontWeight: 500 }}>
          Workforce roster is dynamically derived from repair activity in the
          past 30 days. Technicians automatically appear once they claim or
          resolve a repair in the queue.
        </Typography>
      </Box>

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
            High-impact score restored (today)
          </Typography>
        </Paper>

        {/* Avg Queue Pickup Rank */}
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

        {/* In-Progress Repairs */}
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
              <TableCell sx={{ fontWeight: 700, color: 'text.secondary' }}>
                Technician
              </TableCell>
              <TableCell sx={{ fontWeight: 700, color: 'text.secondary' }}>
                Currently Claimed DUTs (In-Progress)
              </TableCell>
              <TableCell
                align="center"
                sx={{ fontWeight: 700, color: 'text.secondary' }}
              >
                Priority Score Cleared ({timeframe})
              </TableCell>
              <TableCell
                align="center"
                sx={{ fontWeight: 700, color: 'text.secondary' }}
              >
                <Box
                  sx={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 0.5,
                  }}
                >
                  <span>Avg Queue Pickup Rank ({timeframe})</span>
                  <Tooltip title="Average position of tasks in the priority queue when claimed by the technician.">
                    <HelpOutlineIcon
                      sx={{ fontSize: 16, color: 'text.secondary' }}
                    />
                  </Tooltip>
                </Box>
              </TableCell>
              <TableCell
                align="center"
                sx={{ fontWeight: 700, color: 'text.secondary' }}
              >
                Completed Repairs ({timeframe})
              </TableCell>
              <TableCell
                align="center"
                sx={{ fontWeight: 700, color: 'text.secondary' }}
              >
                Avg Duration (MTTR)
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell
                  colSpan={6}
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
                  colSpan={6}
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
                  colSpan={6}
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
                  {/* Technician */}
                  <TableCell>
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}
                    >
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
                  </TableCell>

                  {/* Currently Claimed DUTs */}
                  <TableCell>
                    {tech.claimedDuts.length > 0 ? (
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                        {tech.claimedDuts.map((dut) => (
                          <Chip
                            key={dut.dutId}
                            icon={<PersonIcon sx={{ fontSize: 16 }} />}
                            label={`${dut.dutId} (Score: ${dut.score} | ${formatDuration(dut.duration)})`}
                            variant="outlined"
                            color="primary"
                            size="small"
                            onDelete={() =>
                              handleUnclaimDut(tech.id, dut.dutId)
                            }
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
                    )}
                  </TableCell>

                  {/* Priority Score Cleared */}
                  <TableCell align="center">
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
                  </TableCell>

                  {/* Avg Queue Pickup Rank */}
                  <TableCell align="center">
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
                  </TableCell>

                  {/* Completed Repairs */}
                  <TableCell align="center">
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>
                      {tech.completedRepairs} devices
                    </Typography>
                  </TableCell>

                  {/* Avg Duration (MTTR) */}
                  <TableCell align="center">
                    <Typography variant="body2" color="text.secondary">
                      {formatDuration(tech.avgDuration)}
                    </Typography>
                  </TableCell>
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
