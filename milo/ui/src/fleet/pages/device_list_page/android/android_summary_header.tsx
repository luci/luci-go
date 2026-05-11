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

import CheckIcon from '@mui/icons-material/Check';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import {
  Alert,
  Box,
  Grid,
  Typography,
  Divider,
  Link,
  Skeleton,
  ButtonBase,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidState } from './android_state';

export interface AndroidSummaryHeaderProps {
  aip160: string;
  setFiltersBatch: (updates: Record<string, string[]>) => void;
}

const COLUMN_STYLE = {
  padding: '2px 4px',
};

// Centralized filter keys to avoid hardcoding quotes everywhere
const FILTER_KEYS = {
  STATE: '"state"',
  MACHINE_TYPE: '"fc_machine_type"',
} as const;

interface SmallMetricItemProps {
  label: string | React.ReactNode;
  value: number;
  total?: number;
  onClick?: () => void;
  dotColor?: string;
  loading?: boolean;
}

// Moved outside to avoid remounting anti-pattern
// Uses ButtonBase for better accessibility and native keyboard support
function SmallMetricItem({
  label,
  value,
  total,
  onClick,
  dotColor,
  loading,
}: SmallMetricItemProps) {
  const theme = useTheme();
  const percentage =
    total && total > 0 ? ((value / total) * 100).toFixed(1) : undefined;

  return (
    <ButtonBase
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 0.5,
        cursor: onClick ? 'pointer' : 'default',
        width: '100%',
        justifyContent: 'flex-start',
        textAlign: 'left',
        '&:hover': {
          backgroundColor: theme.palette.action.hover,
          borderRadius: '4px',
        },
        // Target specific class to avoid over-broad styling
        '&:hover .small-metric-label': onClick
          ? { textDecoration: 'underline' }
          : {},
        p: '1px 4px',
        minHeight: '1rem',
      }}
    >
      {dotColor && (
        <Box
          sx={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: dotColor,
            flexShrink: 0,
            mr: 0.5,
          }}
        />
      )}
      <Typography
        component="div"
        variant="body2"
        className="small-metric-label"
        sx={{
          color: 'text.secondary',
          fontSize: '0.85rem',
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
        }}
      >
        {label}
      </Typography>
      {loading ? (
        <Skeleton width={30} height={16} sx={{ ml: 0.5 }} />
      ) : (
        <Typography
          variant="body2"
          sx={{
            color: 'text.primary',
            fontWeight: 700,
            fontSize: '0.85rem',
            ml: 0.5,
          }}
        >
          {value.toLocaleString()}
        </Typography>
      )}
      {percentage !== undefined && !loading && (
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
            fontWeight: 300,
            fontSize: '0.75rem',
            opacity: 0.7,
            ml: 0.5,
          }}
        >
          ({percentage}%)
        </Typography>
      )}
    </ButtonBase>
  );
}

export function AndroidSummaryHeader({
  aip160,
  setFiltersBatch,
}: AndroidSummaryHeaderProps) {
  const client = useFleetConsoleClient();
  const theme = useTheme();

  const BORDER_STYLE = `1px solid ${theme.palette.divider}`;

  const colors = {
    emerald: theme.palette.success.main,
    rose: theme.palette.error.main,
    amber: theme.palette.warning.main,
    slate: theme.palette.text.secondary,
    grey: theme.palette.grey[400],
    dark: theme.palette.text.primary,
  };

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: aip160,
      platform: Platform.ANDROID,
    }),
  );

  const data = countQuery.data?.androidCount;
  const totalDevices = data?.totalDevices || 0;
  const totalHosts = data?.totalHosts || 0;

  // Guard against flashing 0 counts by checking data availability
  const isLoading = countQuery.isPending || !data;

  const recoveringCount = isLoading
    ? undefined
    : (data?.initDevices || 0) +
      (data?.dirtyDevices || 0) +
      (data?.preppingDevices || 0) +
      (data?.lameduckDevices || 0);

  const totalHealthyDevices = isLoading
    ? undefined
    : (data?.idleDevices || 0) +
      (data?.busyDevices || 0) +
      (recoveringCount || 0);

  const totalUnhealthyDevices = isLoading
    ? undefined
    : (data?.missingDevices || 0) +
      (data?.failedDevices || 0) +
      (data?.dyingDevices || 0);

  // Enforce lower boundary to avoid negative rendering during sync delays
  const totalOtherDevices = isLoading
    ? undefined
    : Math.max(
        0,
        totalDevices -
          (totalHealthyDevices || 0) -
          (totalUnhealthyDevices || 0),
      );

  const unclassifiedDevices = totalOtherDevices;

  // TODO(b/503171080): CountDevices API returns 0 for (Blank) state searches.
  // Investigate why backend aggregation fails for blank states.

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
      );
    }

    return (
      <Box sx={{ p: 1 }}>
        {/* Top Row: Hosts Health */}
        <Grid container spacing={0} alignItems="stretch">
          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{
              ...COLUMN_STYLE,
              borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Total Hosts"
              value={totalHosts}
              loading={isLoading}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.MACHINE_TYPE]: ['host'],
                  [FILTER_KEYS.STATE]: [],
                })
              }
            />
          </Grid>
          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{
              ...COLUMN_STYLE,
              borderRight: { md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Hosts Running"
              value={data?.labRunningHosts || 0}
              total={totalHosts}
              loading={isLoading}
              Icon={<CheckIcon sx={{ color: colors.emerald }} />}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.STATE]: [androidState.LAB_RUNNING],
                  [FILTER_KEYS.MACHINE_TYPE]: ['host'],
                })
              }
            />
          </Grid>
          <Divider
            sx={{
              display: { xs: 'block', md: 'none' },
              width: '100%',
              my: 0.5,
              borderColor: 'rgba(0, 0, 0, 0.05)',
            }}
          />
          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{
              ...COLUMN_STYLE,
              borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Hosts Missing"
              value={data?.labMissingHosts || 0}
              total={totalHosts}
              loading={isLoading}
              Icon={<ErrorIcon sx={{ color: colors.rose }} />}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.STATE]: [androidState.LAB_MISSING],
                  [FILTER_KEYS.MACHINE_TYPE]: ['host'],
                })
              }
            />
          </Grid>
          {/* Spacer visible on sm (2 columns) to maintain grid lines */}
          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{
              ...COLUMN_STYLE,
              display: { xs: 'none', sm: 'block', md: 'block' },
            }}
          />
        </Grid>

        <Divider sx={{ my: 0.5, borderColor: 'rgba(0, 0, 0, 0.05)' }} />

        {/* Bottom Row: Devices Health */}
        <Grid container spacing={0} alignItems="stretch">
          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{
              ...COLUMN_STYLE,
              borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Total Devices"
              value={totalDevices}
              loading={isLoading}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  [FILTER_KEYS.STATE]: [],
                })
              }
            />
          </Grid>

          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            aria-label="Healthy Devices"
            sx={{
              ...COLUMN_STYLE,
              borderRight: { md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Total Healthy"
              value={totalHealthyDevices}
              total={totalDevices}
              loading={isLoading}
              Icon={<CheckIcon sx={{ color: colors.emerald }} />}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.STATE]: [
                    androidState.IDLE,
                    androidState.BUSY,
                    androidState.INIT,
                    androidState.DIRTY,
                    androidState.PREPPING,
                    androidState.LAMEDUCK,
                  ],
                  [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                })
              }
            />
            <Box
              sx={{
                mt: 0.5,
                px: 1,
                display: 'flex',
                flexDirection: 'column',
                gap: 0,
              }}
            >
              <SmallMetricItem
                label="Idle:"
                value={data?.idleDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [androidState.IDLE],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.emerald}
                loading={isLoading}
              />
              <SmallMetricItem
                label="Busy:"
                value={data?.busyDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [androidState.BUSY],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.emerald}
                loading={isLoading}
              />
              <SmallMetricItem
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    Recovering:
                    <InfoTooltip>
                      <Box sx={{ p: 0.5 }}>
                        <Typography variant="body2">
                          Counted as healthy because these devices are expected
                          to recover automatically (Init, Dirty, Prepping,
                          Lameduck).
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 2 }}>
                          <Link
                            href="http://go/android-labtechs#device-terminology"
                            target="_blank"
                            rel="noopener noreferrer"
                            aria-label="documentation (opens in a new tab)"
                          >
                            See documentation for details.
                          </Link>
                        </Typography>
                      </Box>
                    </InfoTooltip>
                  </Box>
                }
                value={recoveringCount || 0}
                total={totalDevices}
                dotColor={colors.grey}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [
                      androidState.INIT,
                      androidState.DIRTY,
                      androidState.PREPPING,
                      androidState.LAMEDUCK,
                    ],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                loading={isLoading}
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  label="Init:"
                  value={data?.initDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.INIT],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Dirty:"
                  value={data?.dirtyDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.DIRTY],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Prepping:"
                  value={data?.preppingDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.PREPPING],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Lameduck:"
                  value={data?.lameduckDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.LAMEDUCK],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
              </Box>
            </Box>
          </Grid>

          <Divider
            sx={{
              display: { xs: 'block', md: 'none' },
              width: '100%',
              my: 0.5,
              borderColor: 'rgba(0, 0, 0, 0.05)',
            }}
          />

          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            aria-label="Unhealthy Devices"
            sx={{
              ...COLUMN_STYLE,
              borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, md: 'none' },
            }}
          >
            <SingleMetric
              name="Total Unhealthy"
              value={totalUnhealthyDevices}
              total={totalDevices}
              loading={isLoading}
              Icon={<ErrorIcon sx={{ color: colors.rose }} />}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.STATE]: [
                    androidState.MISSING,
                    androidState.FAILED,
                    androidState.DYING,
                  ],
                  [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                })
              }
            />
            <Box
              sx={{
                mt: 0.5,
                px: 1,
                display: 'flex',
                flexDirection: 'column',
                gap: 0,
              }}
            >
              <SmallMetricItem
                label="Missing:"
                value={data?.missingDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [androidState.MISSING],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.rose}
                loading={isLoading}
              />
              <SmallMetricItem
                label="Failed:"
                value={data?.failedDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [androidState.FAILED],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.rose}
                loading={isLoading}
              />
              <SmallMetricItem
                label="Dying:"
                value={data?.dyingDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: [androidState.DYING],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.rose}
                loading={isLoading}
              />
            </Box>
          </Grid>

          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            aria-label="Other Devices"
            sx={{
              ...COLUMN_STYLE,
              borderBottom: { xs: BORDER_STYLE, sm: 'none', md: 'none' },
            }}
          >
            <SingleMetric
              name="Total Other"
              value={totalOtherDevices}
              total={totalDevices}
              loading={isLoading}
              Icon={<WarningIcon sx={{ color: colors.amber }} />}
              handleClick={() =>
                setFiltersBatch({
                  [FILTER_KEYS.STATE]: ['(Blank)'],
                  [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                })
              }
            />
            <Box
              sx={{
                mt: 0.5,
                px: 1,
                display: 'flex',
                flexDirection: 'column',
                gap: 0,
              }}
            >
              <SmallMetricItem
                label="Unclassified:"
                value={unclassifiedDevices || 0}
                total={totalDevices}
                onClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.STATE]: ['(Blank)'],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
                dotColor={colors.amber}
                loading={isLoading}
              />
            </Box>
          </Grid>
        </Grid>
      </Box>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h6" sx={{ mb: 2, color: colors.dark }}>
        Device Health Metrics
      </Typography>
      {getContent()}
    </MetricsContainer>
  );
}
