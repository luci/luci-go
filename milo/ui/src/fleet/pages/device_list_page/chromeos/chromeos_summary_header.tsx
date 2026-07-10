// Copyright 2025 The LUCI Authors.
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
import { Alert, Box, Divider, Grid, Link, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { SmallMetricItem } from '@/fleet/components/summary_header/small_metric_item';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { getMetricsGridStyles } from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors as fleetColors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { parseChromeosDeviceMetrics } from '@/fleet/utils/metrics';
import { combineAipFilters, formatAipClause } from '@/fleet/utils/search_param';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

// Recommended filters for finding all CrOS labstations. See: b/398911822#comment4
const LABSTATION_FILTERS: Record<string, string[]> = {
  'labels."label-pool"': [
    'labstation_tryjob',
    'labstation_main',
    'labstation_canary',
  ],
};

const OTHER_DUT_STATES = [
  'needs_deploy',
  'needs_replacement',
  'registered',
  'reserved',
  'unknown',
  BLANK_VALUE,
];

const AVAILABLE_READY_FILTERS: Record<string, string[]> = {
  state: ['DEVICE_STATE_AVAILABLE'],
  'labels."dut_state"': ['ready'],
};

const NOT_READY_DUT_STATES = [
  'needs_repair',
  'repair_failed',
  'needs_manual_repair',
  'needs_deploy',
  'needs_replacement',
  'registered',
  'reserved',
  'unknown',
];

// Static filter objects extracted to avoid re-renders.
const AVAILABLE_FILTERS: Record<string, string[]> = {
  state: ['DEVICE_STATE_AVAILABLE'],
  'labels."dut_state"': [],
};

const LEASED_FILTERS: Record<string, string[]> = {
  state: ['DEVICE_STATE_LEASED'],
  'labels."dut_state"': [],
};

export interface ChromeOSSummaryHeaderProps {
  aip160: string;
  setFiltersBatch: (updates: Record<string, string[]>) => void;
}

/**
 * ChromeOSSummaryHeader displays the main metrics section for ChromeOS devices.
 */
export function ChromeOSSummaryHeader({
  aip160,
  setFiltersBatch,
}: ChromeOSSummaryHeaderProps) {
  const client = useFleetConsoleClient();
  const theme = useTheme();

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: aip160,
      platform: Platform.CHROMEOS,
    }),
  );

  const labstationClause = formatAipClause('labels."label-pool"', [
    'labstation_tryjob',
    'labstation_main',
    'labstation_canary',
  ]);
  const labstationsFilter = combineAipFilters(aip160, labstationClause);

  const labstationsQuery = useQuery({
    ...client.CountDevices.query({
      filter: labstationsFilter,
      platform: Platform.CHROMEOS,
    }),
  });

  const availableReadyFilter =
    'state = "DEVICE_STATE_AVAILABLE" AND labels."dut_state" = "ready"';

  const availableReadyQuery = useQuery({
    ...client.CountDevices.query({
      filter: [aip160, availableReadyFilter].filter(Boolean).join(' AND '),
      platform: Platform.CHROMEOS,
    }),
  });

  const colors = {
    success: theme.palette.success.main,
    error: theme.palette.error.main,
    warning: theme.palette.warning.main,
    grey: theme.palette.grey[400],
    dark: theme.palette.text.primary,
    reserved: fleetColors.purple[400],
  };

  const getContent = () => {
    if (
      countQuery.isError ||
      labstationsQuery.isError ||
      availableReadyQuery.isError
    ) {
      return (
        <Alert severity="error">
          {getErrorMessage(
            countQuery.error ||
              labstationsQuery.error ||
              availableReadyQuery.error,
            'get the main metrics',
          )}
        </Alert>
      );
    }

    const {
      total: totalLabstations,
      ready: labstationsReady,
      needRepair: labstationsNeedRepair,
      repairFailed: labstationsRepairFailed,
      recovering: labstationsRecovering,
      healthy: labstationsHealthy,
      needManualRepair: labstationsNeedManualRepair,
      unhealthy: labstationsUnhealthy,
      needsDeploy: labstationsNeedsDeploy,
      needsReplacement: labstationsNeedsReplacement,
      other: labstationsOther,
      otherStates: labstationsOtherStates,
    } = parseChromeosDeviceMetrics(
      labstationsQuery.data?.deviceState,
      labstationsQuery.data?.total || 0,
    );

    const {
      total: totalDevices,
      ready: devicesReady,
      needRepair: devicesNeedRepair,
      repairFailed: devicesRepairFailed,
      recovering: devicesRecovering,
      healthy: devicesHealthy,
      needManualRepair: devicesNeedManualRepair,
      unhealthy: devicesUnhealthy,
      needsDeploy: devicesNeedsDeploy,
      needsReplacement: devicesNeedsReplacement,
      reserved: devicesReserved,
      other: devicesOther,
      otherStatesExcludingReserved: devicesOtherStatesExcludingReserved,
    } = parseChromeosDeviceMetrics(
      countQuery.data?.deviceState,
      countQuery.data?.total || 0,
    );

    const isLoading =
      countQuery.isPending ||
      labstationsQuery.isPending ||
      availableReadyQuery.isPending;
    const gridStyles = getMetricsGridStyles(theme);

    const handleMetricClick = (newFilters: Record<string, string[]>) => {
      setFiltersBatch(newFilters);
    };

    const availableIdle = countQuery.data?.taskState?.idle || 0;
    const availableReady = availableReadyQuery.data?.total || 0;
    // Note: These queries execute independently and may reflect slightly different
    // timestamps (race condition), causing availableReady to occasionally exceed availableIdle.
    const notReadyValue = availableIdle - availableReady;
    if (notReadyValue < 0) {
      // eslint-disable-next-line no-console
      console.warn(
        `Data anomaly: availableReady (${availableReady}) is greater than availableIdle (${availableIdle})`,
      );
    }

    return (
      <Box sx={{ p: 1 }}>
        {/* Labstations Row */}
        <Grid container spacing={0} alignItems="stretch">
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col1}>
            <SingleMetric
              name="Total Labstations"
              value={totalLabstations}
              loading={isLoading}
              handleClick={() =>
                handleMetricClick({
                  ...LABSTATION_FILTERS,
                  'labels."dut_state"': [],
                })
              }
            />
          </Grid>
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col2}>
            <SingleMetric
              name="Healthy"
              value={labstationsHealthy}
              total={totalLabstations}
              loading={isLoading}
              Icon={<CheckIcon sx={{ color: colors.success }} />}
              handleClick={() =>
                handleMetricClick({
                  ...LABSTATION_FILTERS,
                  'labels."dut_state"': [
                    'ready',
                    'needs_repair',
                    'repair_failed',
                  ],
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Ready:"
                value={labstationsReady}
                total={totalLabstations}
                dotColor={colors.success}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['ready'],
                  })
                }
              />
              <SmallMetricItem
                label="Recovering:"
                value={labstationsRecovering}
                total={totalLabstations}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['needs_repair', 'repair_failed'],
                  })
                }
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  label="Need repair:"
                  value={labstationsNeedRepair}
                  total={totalLabstations}
                  dotColor={colors.grey}
                  loading={isLoading}
                  onClick={() =>
                    handleMetricClick({
                      ...LABSTATION_FILTERS,
                      'labels."dut_state"': ['needs_repair'],
                    })
                  }
                />
                <SmallMetricItem
                  label="Repair failed:"
                  value={labstationsRepairFailed}
                  total={totalLabstations}
                  dotColor={colors.grey}
                  loading={isLoading}
                  onClick={() =>
                    handleMetricClick({
                      ...LABSTATION_FILTERS,
                      'labels."dut_state"': ['repair_failed'],
                    })
                  }
                />
              </Box>
            </Box>
          </Grid>

          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col3}>
            <SingleMetric
              name="Unhealthy"
              value={labstationsUnhealthy}
              total={totalLabstations}
              loading={isLoading}
              Icon={<ErrorIcon sx={{ color: colors.error }} />}
              handleClick={() =>
                handleMetricClick({
                  ...LABSTATION_FILTERS,
                  'labels."dut_state"': ['needs_manual_repair'],
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Need manual repair:"
                value={labstationsNeedManualRepair}
                total={totalLabstations}
                dotColor={colors.error}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['needs_manual_repair'],
                  })
                }
              />
            </Box>
          </Grid>
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col4}>
            <SingleMetric
              name="Other"
              value={labstationsOther}
              total={totalLabstations}
              loading={isLoading}
              Icon={<WarningIcon sx={{ color: colors.warning }} />}
              handleClick={() =>
                handleMetricClick({
                  ...LABSTATION_FILTERS,
                  'labels."dut_state"': OTHER_DUT_STATES,
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Needs deploy:"
                value={labstationsNeedsDeploy}
                total={totalLabstations}
                dotColor={colors.warning}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['needs_deploy'],
                  })
                }
              />
              <SmallMetricItem
                label="Needs replacement:"
                value={labstationsNeedsReplacement}
                total={totalLabstations}
                dotColor={colors.warning}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['needs_replacement'],
                  })
                }
              />
              <SmallMetricItem
                label="Other states:"
                value={labstationsOtherStates}
                total={totalLabstations}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    ...LABSTATION_FILTERS,
                    'labels."dut_state"': ['unknown', 'registered', 'reserved'],
                  })
                }
              />
            </Box>
          </Grid>
        </Grid>

        <Divider sx={{ my: 2, borderColor: 'rgba(0, 0, 0, 0.05)' }} />

        {/* Devices Row */}
        <Grid
          container
          spacing={0}
          alignItems="stretch"
          data-testid="devices-metrics-grid"
        >
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col1}>
            <SingleMetric
              name="Total Devices"
              value={totalDevices}
              loading={isLoading}
              handleClick={() =>
                handleMetricClick({
                  'labels."dut_state"': [],
                  'labels."label-pool"': [],
                  state: [],
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Available:"
                value={countQuery.data?.taskState?.idle || 0}
                total={totalDevices}
                dotColor={colors.success}
                loading={isLoading}
                onClick={() => handleMetricClick(AVAILABLE_FILTERS)}
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  ariaLabel="Ready"
                  label={
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
                      Ready:
                      <InfoTooltip>
                        <Box sx={{ p: 0.5 }}>
                          <Typography variant="body2">
                            Devices that are <strong>Ready</strong> and{' '}
                            <strong>Available</strong> can immediately take on
                            new tests. Both states are required to understand
                            lab capacity as <strong>Ready</strong> devices are
                            healthy but might be leased, while{' '}
                            <strong>Available</strong> devices could be able to
                            run tasks (i.e., repair tasks) but are broken.
                          </Typography>
                        </Box>
                      </InfoTooltip>
                    </Box>
                  }
                  value={availableReady}
                  total={totalDevices}
                  dotColor={colors.success}
                  loading={isLoading}
                  onClick={() => handleMetricClick(AVAILABLE_READY_FILTERS)}
                />
                <SmallMetricItem
                  ariaLabel="Not ready"
                  label={
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
                      Not ready:
                      <InfoTooltip>
                        <Box sx={{ p: 0.5 }}>
                          <Typography variant="body2">
                            Devices that are <strong>Available</strong> but{' '}
                            <strong>Not ready</strong> might be broken but able
                            to run repair jobs or similar.
                          </Typography>
                        </Box>
                      </InfoTooltip>
                    </Box>
                  }
                  value={Math.max(0, notReadyValue)}
                  total={totalDevices}
                  dotColor={colors.grey}
                  loading={isLoading}
                  onClick={() =>
                    handleMetricClick({
                      state: ['DEVICE_STATE_AVAILABLE'],
                      'labels."dut_state"': NOT_READY_DUT_STATES,
                    })
                  }
                />
              </Box>
              <SmallMetricItem
                label="Leased:"
                value={countQuery.data?.taskState?.busy || 0}
                total={totalDevices}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() => handleMetricClick(LEASED_FILTERS)}
              />
            </Box>
          </Grid>
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col2}>
            <SingleMetric
              name="Total Healthy"
              value={devicesHealthy}
              total={totalDevices}
              loading={isLoading}
              Icon={<CheckIcon sx={{ color: colors.success }} />}
              handleClick={() =>
                handleMetricClick({
                  'labels."dut_state"': [
                    'ready',
                    'needs_repair',
                    'repair_failed',
                  ],
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Ready:"
                value={devicesReady}
                total={totalDevices}
                dotColor={colors.success}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({ 'labels."dut_state"': ['ready'] })
                }
              />
              <SmallMetricItem
                ariaLabel="Recovering"
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    Recovering:
                    <InfoTooltip>
                      <Box sx={{ p: 0.5 }}>
                        <Typography variant="body2">
                          Devices in recovering state are expected to be fixed
                          by automated repair jobs.
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 2 }}>
                          <Link
                            href="http://go/flops-swarming#dut-states"
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
                value={devicesRecovering}
                total={totalDevices}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['needs_repair', 'repair_failed'],
                  })
                }
              />
              <Box sx={{ ml: 2 }}>
                <SmallMetricItem
                  label="Need repair:"
                  value={devicesNeedRepair}
                  total={totalDevices}
                  dotColor={colors.grey}
                  loading={isLoading}
                  onClick={() =>
                    handleMetricClick({
                      'labels."dut_state"': ['needs_repair'],
                    })
                  }
                />
                <SmallMetricItem
                  label="Repair failed:"
                  value={devicesRepairFailed}
                  total={totalDevices}
                  dotColor={colors.grey}
                  loading={isLoading}
                  onClick={() =>
                    handleMetricClick({
                      'labels."dut_state"': ['repair_failed'],
                    })
                  }
                />
              </Box>
            </Box>
          </Grid>
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col3}>
            <SingleMetric
              name="Total Unhealthy"
              value={devicesUnhealthy}
              total={totalDevices}
              loading={isLoading}
              Icon={<ErrorIcon sx={{ color: colors.error }} />}
              handleClick={() =>
                handleMetricClick({
                  'labels."dut_state"': ['needs_manual_repair'],
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Need manual repair:"
                value={devicesNeedManualRepair}
                total={totalDevices}
                dotColor={colors.error}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['needs_manual_repair'],
                  })
                }
              />
            </Box>
          </Grid>
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col4}>
            <SingleMetric
              name="Total Other"
              value={devicesOther}
              total={totalDevices}
              loading={isLoading}
              Icon={<WarningIcon sx={{ color: colors.warning }} />}
              handleClick={() =>
                handleMetricClick({
                  'labels."dut_state"': OTHER_DUT_STATES,
                })
              }
            />
            <Box
              sx={{ mt: 0.5, px: 1, display: 'flex', flexDirection: 'column' }}
            >
              <SmallMetricItem
                label="Needs deploy:"
                value={devicesNeedsDeploy}
                total={totalDevices}
                dotColor={colors.warning}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({ 'labels."dut_state"': ['needs_deploy'] })
                }
              />
              <SmallMetricItem
                label="Needs replacement:"
                value={devicesNeedsReplacement}
                total={totalDevices}
                dotColor={colors.warning}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['needs_replacement'],
                  })
                }
              />
              <SmallMetricItem
                label="Reserved:"
                value={devicesReserved}
                total={totalDevices}
                dotColor={colors.reserved}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['reserved'],
                  })
                }
              />
              <SmallMetricItem
                label="Other states:"
                value={devicesOtherStatesExcludingReserved}
                total={totalDevices}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['unknown', 'registered'],
                  })
                }
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
