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
import { Alert, Box, Grid, Typography, Divider, Link } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { SmallMetricItem } from '@/fleet/components/summary_header/small_metric_item';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import {
  METRICS_COLUMN_STYLE,
  getMetricsGridStyles,
} from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';
import { combineAipFilters } from '@/fleet/utils/search_param';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { androidState } from './android_state';

export interface AndroidSummaryHeaderProps {
  aip160: string;
  setFiltersBatch: (updates: Record<string, string[]>) => void;
  showAvgUtilization?: boolean;
}

// Centralized filter keys to avoid hardcoding quotes everywhere
const FILTER_KEYS = {
  STATE: '"state"',
  MACHINE_TYPE: '"fc_machine_type"',
  FC_IS_OFFLINE: '"fc_is_offline"',
} as const;

export function AndroidSummaryHeader({
  aip160,
  setFiltersBatch,
  showAvgUtilization = false,
}: AndroidSummaryHeaderProps) {
  const client = useFleetConsoleClient();
  const theme = useTheme();

  const BORDER_STYLE = `1px solid ${theme.palette.divider}`;
  const gridStyles = getMetricsGridStyles(theme);

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

  const onlineFilter = combineAipFilters(aip160, 'fc_is_offline = "false"');
  const offlineFilter = combineAipFilters(aip160, 'fc_is_offline = "true"');

  // TODO(b/512174685): Optimize backend CountDevices to return online/offline breakdown in a single payload to reduce traffic.
  const onlineCountQuery = useQuery(
    client.CountDevices.query({
      filter: onlineFilter,
      platform: Platform.ANDROID,
    }),
  );

  const offlineCountQuery = useQuery(
    client.CountDevices.query({
      filter: offlineFilter,
      platform: Platform.ANDROID,
    }),
  );

  const onlineData = onlineCountQuery.data?.androidCount;
  const offlineData = offlineCountQuery.data?.androidCount;

  // Guard against flashing 0 counts by checking data availability
  const isLoading =
    countQuery.isPending ||
    onlineCountQuery.isPending ||
    offlineCountQuery.isPending ||
    !data;

  const onlineDevicesCount = isLoading
    ? undefined
    : onlineData?.totalDevices || 0;

  const failedDeviceType = isLoading
    ? undefined
    : (offlineData?.idleDevices || 0) +
      (offlineData?.busyDevices || 0) +
      (offlineData?.lameduckDevices || 0);

  // In Automation for Offline includes Init, Dirty, Prepping
  const inAutomationOffline = isLoading
    ? undefined
    : (offlineData?.initDevices || 0) +
      (offlineData?.dirtyDevices || 0) +
      (offlineData?.preppingDevices || 0);

  const blankStatesCount =
    isLoading || !onlineData
      ? undefined
      : Math.max(
          0,
          onlineData.totalDevices -
            (onlineData.idleDevices || 0) -
            (onlineData.busyDevices || 0) -
            (onlineData.lameduckDevices || 0),
        );

  // TODO(b/503171080): CountDevices API returns 0 for (Blank) state searches.
  // Investigate why backend aggregation fails for blank states.

  const getContent = () => {
    const isError =
      countQuery.isError ||
      onlineCountQuery.isError ||
      offlineCountQuery.isError;
    const error =
      countQuery.error || onlineCountQuery.error || offlineCountQuery.error;

    if (isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(error, 'get the metrics')}
        </Alert>
      );
    }

    return (
      <Box sx={{ p: 1 }}>
        {/* Top Row: Hosts Health */}
        <Grid container spacing={0} alignItems="stretch">
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col1}>
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
          <Grid item xs={12} sm={6} md={3} sx={gridStyles.col2}>
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

          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            sx={{ ...gridStyles.col3, borderRight: 'none' }}
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
              ...METRICS_COLUMN_STYLE,
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
              ...METRICS_COLUMN_STYLE,
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
            {showAvgUtilization && (
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  mt: 0.5,
                  px: 1,
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  <Typography variant="body2">Utilization</Typography>
                  <InfoTooltip>
                    <Typography variant="body2">
                      Average or all the average utilizations matching the
                      current search.
                      <br />
                      Devices with missing utilization data are excluded from
                      this metric.
                    </Typography>
                  </InfoTooltip>
                </Box>
                <Box
                  sx={{
                    mt: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 0.5,
                  }}
                >
                  <SmallMetricItem
                    label="7 days avg:"
                    value={data?.average7d}
                    loading={isLoading}
                    formatValue={(val) =>
                      data?.average7d === undefined ? '-' : `${val.toFixed(2)}%`
                    }
                  />
                  <SmallMetricItem
                    label="30 days avg:"
                    value={data?.average30d}
                    loading={isLoading}
                    formatValue={(val) =>
                      data?.average30d === undefined
                        ? '-'
                        : `${val.toFixed(2)}%`
                    }
                  />
                </Box>
              </Box>
            )}
          </Grid>

          <Grid
            item
            xs={12}
            sm={6}
            md={3}
            aria-label="Online Devices"
            sx={{
              ...METRICS_COLUMN_STYLE,
              borderRight: { md: BORDER_STYLE },
              borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Online"
                value={onlineDevicesCount}
                total={totalDevices}
                loading={isLoading}
                Icon={<CheckIcon sx={{ color: colors.emerald }} />}
                handleClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
              />
              <InfoTooltip>
                <Typography variant="body2">
                  Devices considered healthy per{' '}
                  <Link
                    href="https://g3doc.corp.google.com/company/teams/omnilab/products/lmp/monitoring/alert_config_manual.md?cl=head#Q3"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Omnilab&apos;s definition
                  </Link>
                  . This status is tracked by the fc_is_offline field.
                  <br />
                  <br />
                  Note: Percentages of breakdown items may not sum to the total
                  due to rounding.
                </Typography>
              </InfoTooltip>
            </Box>
            <Grid container spacing={1} sx={{ mt: 0.5, px: 1 }}>
              <Grid item xs={12}>
                <SmallMetricItem
                  label="Idle:"
                  value={onlineData?.idleDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.IDLE],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.emerald}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Busy:"
                  value={onlineData?.busyDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.BUSY],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.emerald}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Lameduck:"
                  value={onlineData?.lameduckDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.LAMEDUCK],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Blank states:"
                  value={blankStatesCount}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: ['(Blank)'],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['false'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.grey}
                  loading={isLoading}
                />
              </Grid>
            </Grid>
          </Grid>

          {/* Offline Devices takes md={6} to accommodate two internal sub-columns (Needs Action and Recovering) */}
          <Grid
            item
            xs={12}
            sm={12}
            md={6}
            aria-label="Offline Devices"
            sx={{
              ...METRICS_COLUMN_STYLE,
              borderRight: {
                sm: 'none',
                md: 'none',
              },
              borderBottom: {
                xs: 'none',
                sm: 'none',
                md: 'none',
              },
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <SingleMetric
                name="Offline"
                value={offlineData?.totalDevices || 0}
                total={totalDevices}
                loading={isLoading}
                Icon={<ErrorIcon sx={{ color: colors.rose }} />}
                handleClick={() =>
                  setFiltersBatch({
                    [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                    [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                  })
                }
              />
              <InfoTooltip>
                <Typography variant="body2">
                  Devices considered unhealthy per{' '}
                  <Link
                    href="https://g3doc.corp.google.com/company/teams/omnilab/products/lmp/monitoring/alert_config_manual.md?cl=head#Q3"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Omnilab&apos;s definition
                  </Link>
                  . This status is tracked by the fc_is_offline field.
                  <br />
                  <br />
                  Note: Percentages of breakdown items may not sum to the total
                  due to rounding.
                </Typography>
              </InfoTooltip>
            </Box>
            <Grid container spacing={1} sx={{ mt: 0.5, px: 1 }}>
              <Grid item xs={12} md={7}>
                <SmallMetricItem
                  label="Missing:"
                  value={offlineData?.missingDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.MISSING],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.rose}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Failed:"
                  value={offlineData?.failedDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.FAILED],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.rose}
                  loading={isLoading}
                />
                <SmallMetricItem
                  label="Dying:"
                  value={offlineData?.dyingDevices || 0}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.DYING],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.rose}
                  loading={isLoading}
                />
                <SmallMetricItem
                  ariaLabel="Failed device_type"
                  label={
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
                      Failed device_type:
                      <InfoTooltip>
                        <Typography variant="body2">
                          Devices in IDLE, BUSY, or LAMEDUCK status but marked
                          Offline because they have an unhealthy device type
                          (e.g. FailedDevice). See{' '}
                          <Link
                            href="https://g3doc.corp.google.com/company/teams/omnilab/products/lmp/monitoring/alert_config_manual.md?cl=head#Q3"
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Omnilab Definition
                          </Link>
                          .
                        </Typography>
                      </InfoTooltip>
                    </Box>
                  }
                  value={failedDeviceType}
                  total={totalDevices}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [
                        androidState.IDLE,
                        androidState.BUSY,
                        androidState.LAMEDUCK,
                      ],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  dotColor={colors.rose}
                  loading={isLoading}
                />
              </Grid>
              <Grid item xs={12} md={5}>
                <SmallMetricItem
                  ariaLabel="Recovering"
                  label={
                    <Box
                      sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
                    >
                      Recovering:
                      <InfoTooltip>
                        <Typography variant="body2">
                          These devices are in transitory states and are
                          expected to be recovered by automated jobs. See{' '}
                          <Link
                            href="http://go/android-labtechs#device-terminology"
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            Device Terminology
                          </Link>
                          .
                        </Typography>
                      </InfoTooltip>
                    </Box>
                  }
                  value={inAutomationOffline}
                  total={totalDevices}
                  dotColor={colors.grey}
                  onClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [
                        androidState.INIT,
                        androidState.DIRTY,
                        androidState.PREPPING,
                      ],
                      [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                      [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                    })
                  }
                  loading={isLoading}
                />
                <Box sx={{ ml: 2 }}>
                  <SmallMetricItem
                    label="Init:"
                    value={offlineData?.initDevices || 0}
                    total={totalDevices}
                    onClick={() =>
                      setFiltersBatch({
                        [FILTER_KEYS.STATE]: [androidState.INIT],
                        [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                        [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                      })
                    }
                    dotColor={colors.grey}
                    loading={isLoading}
                  />
                  <SmallMetricItem
                    label="Dirty:"
                    value={offlineData?.dirtyDevices || 0}
                    total={totalDevices}
                    onClick={() =>
                      setFiltersBatch({
                        [FILTER_KEYS.STATE]: [androidState.DIRTY],
                        [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                        [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                      })
                    }
                    dotColor={colors.grey}
                    loading={isLoading}
                  />
                  <SmallMetricItem
                    label="Prepping:"
                    value={offlineData?.preppingDevices || 0}
                    total={totalDevices}
                    onClick={() =>
                      setFiltersBatch({
                        [FILTER_KEYS.STATE]: [androidState.PREPPING],
                        [FILTER_KEYS.FC_IS_OFFLINE]: ['true'],
                        [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                      })
                    }
                    dotColor={colors.grey}
                    loading={isLoading}
                  />
                </Box>
              </Grid>
            </Grid>
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
