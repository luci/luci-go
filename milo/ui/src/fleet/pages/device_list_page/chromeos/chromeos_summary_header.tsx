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
import { Alert, Box, Divider, Grid, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { SmallMetricItem } from '@/fleet/components/summary_header/small_metric_item';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { getMetricsGridStyles } from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

interface DeviceStateData {
  deviceState?: {
    ready?: number;
    needRepair?: number;
    repairFailed?: number;
    needManualRepair?: number;
    needsDeploy?: number;
    needsReplacement?: number;
  };
}

function parseDeviceMetrics(data: DeviceStateData | undefined, total: number) {
  const ready = data?.deviceState?.ready || 0;
  const needRepair = data?.deviceState?.needRepair || 0;
  const repairFailed = data?.deviceState?.repairFailed || 0;
  const recovering = needRepair + repairFailed;
  const healthy = ready + recovering;
  const needManualRepair = data?.deviceState?.needManualRepair || 0;
  const unhealthy = needManualRepair;
  const needsDeploy = data?.deviceState?.needsDeploy || 0;
  const needsReplacement = data?.deviceState?.needsReplacement || 0;

  const other = Math.max(0, total - healthy - unhealthy);
  const otherStates = Math.max(0, other - needsDeploy - needsReplacement);

  return {
    total,
    ready,
    needRepair,
    repairFailed,
    recovering,
    healthy,
    needManualRepair,
    unhealthy,
    needsDeploy,
    needsReplacement,
    other,
    otherStates,
  };
}

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

  const labstationsFilter = aip160
    ? `${aip160} AND ${stringifyFilters(LABSTATION_FILTERS)}`
    : stringifyFilters(LABSTATION_FILTERS);

  const labstationsQuery = useQuery({
    ...client.CountDevices.query({
      filter: labstationsFilter,
      platform: Platform.CHROMEOS,
    }),
  });

  const colors = {
    success: theme.palette.success.main,
    error: theme.palette.error.main,
    warning: theme.palette.warning.main,
    grey: theme.palette.grey[400],
    dark: theme.palette.text.primary,
  };

  const getContent = () => {
    if (countQuery.isError || labstationsQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(
            countQuery.error || labstationsQuery.error,
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
    } = parseDeviceMetrics(
      labstationsQuery.data,
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
      other: devicesOther,
      otherStates: devicesOtherStates,
    } = parseDeviceMetrics(countQuery.data, countQuery.data?.total || 0);

    const isLoading = countQuery.isPending || labstationsQuery.isPending;
    const gridStyles = getMetricsGridStyles(theme);

    const handleMetricClick = (newFilters: Record<string, string[]>) => {
      setFiltersBatch(newFilters);
    };

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
        <Grid container spacing={0} alignItems="stretch">
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col1}>
            <SingleMetric
              name="Total Devices"
              value={totalDevices}
              loading={isLoading}
              handleClick={() =>
                handleMetricClick({
                  'labels."dut_state"': [],
                  'labels."label-pool"': [],
                })
              }
            />
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
                label="Recovering:"
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
                label="Other states:"
                value={devicesOtherStates}
                total={totalDevices}
                dotColor={colors.grey}
                loading={isLoading}
                onClick={() =>
                  handleMetricClick({
                    'labels."dut_state"': ['unknown', 'registered', 'reserved'],
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
