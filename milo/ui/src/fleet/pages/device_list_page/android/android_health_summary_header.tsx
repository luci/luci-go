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

import styled from '@emotion/styled';
import CheckIcon from '@mui/icons-material/Check';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import {
  Alert,
  Box,
  Divider,
  FormControlLabel,
  Grid,
  Switch,
  Typography,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useQuery } from '@tanstack/react-query';
import { useLocalStorage } from 'react-use';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { SmallMetricItem } from '@/fleet/components/summary_header/small_metric_item';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { METRICS_COLUMN_STYLE } from '@/fleet/constants/styles';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';
import { HealthCategoryBucket } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { UtilizationTooltipContent } from './android_fields';
import { androidState } from './android_state';

export interface AndroidHealthSummaryHeaderProps {
  aip160: string;
  setFiltersBatch: (updates: Record<string, string[]>) => void;
  showAvgUtilization?: boolean;
}

const FILTER_KEYS = {
  STATE: '"state"',
  MACHINE_TYPE: '"fc_machine_type"',
  HEALTH_CATEGORY: '"health_category"',
} as const;

export const SHOW_ALL_STATES_STORAGE_KEY =
  'fleet-console:android-health-show-all-states';

const PREFERRED_STATUS_ORDER: string[] = [
  androidState.IDLE,
  androidState.BUSY,
  androidState.LAMEDUCK,
  androidState.MISSING,
  androidState.FAILED,
  androidState.DYING,
  androidState.INIT,
  androidState.DIRTY,
  androidState.PREPPING,
  'BLANK',
];

const HoverMetricsContainer = styled(MetricsContainer)`
  .health-metrics-toggle {
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.2s ease;
  }
  &:hover .health-metrics-toggle,
  &:focus-within .health-metrics-toggle {
    opacity: 1;
    pointer-events: auto;
  }
`;

export function AndroidHealthSummaryHeader({
  aip160,
  setFiltersBatch,
  showAvgUtilization = false,
}: AndroidHealthSummaryHeaderProps) {
  const client = useFleetConsoleClient();
  const theme = useTheme();
  const [showAllStates = false, setShowAllStates] = useLocalStorage<boolean>(
    SHOW_ALL_STATES_STORAGE_KEY,
    false,
  );

  const BORDER_STYLE = `1px solid ${theme.palette.divider}`;

  const colors = {
    emerald: theme.palette.success.main,
    rose: theme.palette.error.main,
    amber: theme.palette.warning.main,
    slate: theme.palette.text.secondary,
    grey: theme.palette.grey[400],
    dark: theme.palette.text.primary,
  };

  const healthQuery = useQuery(
    client.CountAndroidDevices.query({
      filter: aip160,
    }),
  );

  const healthData = healthQuery.data;
  const isLoading = healthQuery.isPending || !healthData;

  const totalDevices = healthData?.totalDevices || 0;
  const totalHosts = healthData?.totalHosts || 0;
  const hostsRunning = healthData?.labRunningHosts || 0;
  const hostsMissing = healthData?.labMissingHosts || 0;
  const avg7d = healthData?.average7d;
  const avg30d = healthData?.average30d;

  const formatStateLabel = (state: string): string => {
    switch (state.toUpperCase()) {
      case androidState.IDLE:
        return 'Idle:';
      case androidState.BUSY:
        return 'Busy:';
      case androidState.LAMEDUCK:
        return 'Lameduck:';
      case androidState.MISSING:
        return 'Missing:';
      case androidState.FAILED:
        return 'Failed:';
      case androidState.DYING:
        return 'Dying:';
      case androidState.INIT:
        return 'Init:';
      case androidState.DIRTY:
        return 'Dirty:';
      case androidState.PREPPING:
        return 'Prepping:';
      case 'BLANK':
        return 'Blank states:';
      default:
        return (
          state.charAt(0).toUpperCase() + state.slice(1).toLowerCase() + ':'
        );
    }
  };

  const renderStatusBreakdown = (
    bucket: HealthCategoryBucket | undefined,
    categoryKeys: string[],
  ) => {
    if (!showAllStates) {
      return null;
    }
    if (isLoading || !bucket?.statusCounts) {
      if (!isLoading) return null;
      return PREFERRED_STATUS_ORDER.map((stateKey) => (
        <SmallMetricItem
          key={stateKey}
          label={formatStateLabel(stateKey)}
          value={undefined}
          total={totalDevices}
          loading={true}
        />
      ));
    }
    const entries = Object.entries(bucket.statusCounts).sort(([a], [b]) => {
      const indexA = PREFERRED_STATUS_ORDER.indexOf(a.toUpperCase());
      const indexB = PREFERRED_STATUS_ORDER.indexOf(b.toUpperCase());
      if (indexA !== -1 && indexB !== -1) return indexA - indexB;
      if (indexA !== -1) return -1;
      if (indexB !== -1) return 1;
      return a.localeCompare(b);
    });

    return entries.map(([stateKey, count]) => (
      <SmallMetricItem
        key={stateKey}
        label={formatStateLabel(stateKey)}
        value={count}
        total={totalDevices}
        onClick={() =>
          setFiltersBatch({
            [FILTER_KEYS.HEALTH_CATEGORY]: categoryKeys,
            [FILTER_KEYS.STATE]: [
              stateKey.toUpperCase() === 'BLANK'
                ? '(Blank)'
                : stateKey.toUpperCase(),
            ],
            [FILTER_KEYS.MACHINE_TYPE]: ['device'],
          })
        }
        loading={isLoading}
      />
    ));
  };

  const renderUtilizationSection = (
    avg7dVal?: number | null,
    avg30dVal?: number | null,
  ) => (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        mt: 0.5,
        mb: 2,
        px: 0.5,
        gap: 0.5,
      }}
    >
      <SmallMetricItem
        ariaLabel="7 days avg"
        label={
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            7 days avg:
            <InfoTooltip paperCss={{ maxWidth: '350px' }}>
              <UtilizationTooltipContent isSummary />
            </InfoTooltip>
          </Box>
        }
        value={avg7dVal ?? undefined}
        loading={isLoading}
        formatValue={(val) =>
          avg7dVal === undefined || avg7dVal === null
            ? '-'
            : `${(val * 100).toFixed(2)}%`
        }
      />
      <SmallMetricItem
        ariaLabel="30 days avg"
        label={
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            30 days avg:
            <InfoTooltip paperCss={{ maxWidth: '350px' }}>
              <UtilizationTooltipContent isSummary />
            </InfoTooltip>
          </Box>
        }
        value={avg30dVal ?? undefined}
        loading={isLoading}
        formatValue={(val) =>
          avg30dVal === undefined || avg30dVal === null
            ? '-'
            : `${(val * 100).toFixed(2)}%`
        }
      />
    </Box>
  );

  const automatedMaintenanceBucket: HealthCategoryBucket = {
    total:
      (healthData?.healthCategoryInTransition?.total || 0) +
      (healthData?.healthCategoryInAutoRecovery?.total || 0) +
      (healthData?.healthCategoryUnspecified?.total || 0),
    statusCounts: (() => {
      const counts: { [key: string]: number } = {};
      const addCounts = (statusCounts?: { [key: string]: number }) => {
        if (!statusCounts) return;
        for (const [k, v] of Object.entries(statusCounts)) {
          counts[k] = (counts[k] || 0) + v;
        }
      };
      addCounts(healthData?.healthCategoryInTransition?.statusCounts);
      addCounts(healthData?.healthCategoryInAutoRecovery?.statusCounts);
      addCounts(healthData?.healthCategoryUnspecified?.statusCounts);
      return counts;
    })(),
    average7d: undefined,
    average30d: undefined,
  };

  const healthBuckets: {
    name: string;
    categoryKeys: string[];
    bucket: HealthCategoryBucket | undefined;
    icon: React.ReactElement;
    tooltipText: React.ReactNode;
  }[] = [
    {
      name: 'In Service',
      categoryKeys: ['HEALTH_CATEGORY_IN_SERVICE'],
      bucket: healthData?.healthCategoryInService,
      icon: <CheckIcon sx={{ color: colors.emerald }} />,
      tooltipText: (
        <Typography variant="body2">
          {/* TODO: Update description with final definition. */}
          Healthy devices actively serving test capacity. These devices are
          either running a test or immediately available to accept one.
        </Typography>
      ),
    },
    {
      name: 'Need Manual Repair',
      categoryKeys: ['HEALTH_CATEGORY_NEED_MANUAL_REPAIR'],
      bucket: healthData?.healthCategoryNeedManualRepair,
      icon: <ErrorIcon sx={{ color: colors.rose }} />,
      tooltipText: (
        <Typography variant="body2">
          {/* TODO: Update description with final definition. */}
          Devices requiring physical human intervention from Lab Ops. These
          devices cannot recover automatically.
        </Typography>
      ),
    },
    {
      name: 'In Automated Maintenance',
      categoryKeys: [
        'HEALTH_CATEGORY_IN_TRANSITION',
        'HEALTH_CATEGORY_IN_AUTO_RECOVERY',
        'HEALTH_CATEGORY_UNSPECIFIED',
      ],
      bucket: automatedMaintenanceBucket,
      icon: <WarningIcon sx={{ color: colors.amber }} />,
      tooltipText: (
        <Typography variant="body2">
          {/* TODO: Update description with final definition. */}
          Temporarily unavailable devices undergoing automatic software
          remediation, provisioning, or state transitions. They do not require
          human intervention and are expected to self-recover to In Service.
        </Typography>
      ),
    },
  ];

  if (healthQuery.isError) {
    return (
      <MetricsContainer>
        <Typography variant="h6" sx={{ mb: 2, color: colors.dark }}>
          Device Health Metrics
        </Typography>
        <Alert severity="error">
          {getErrorMessage(healthQuery.error, 'get the metrics')}
        </Alert>
      </MetricsContainer>
    );
  }

  return (
    <HoverMetricsContainer>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          mb: 1.5,
        }}
      >
        <Typography variant="h6" sx={{ color: colors.dark }}>
          Device Health Metrics
        </Typography>
        <FormControlLabel
          className="health-metrics-toggle"
          control={
            <Switch
              size="small"
              checked={!!showAllStates}
              onChange={(e) => setShowAllStates(e.target.checked)}
              inputProps={{ 'aria-label': 'Show all states' }}
            />
          }
          label={
            <Typography
              variant="body2"
              sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}
            >
              Show all states
            </Typography>
          }
          sx={{ mr: 0 }}
        />
      </Box>
      <Box sx={{ p: 1 }}>
        <Box
          sx={{
            overflowX: { xs: 'visible', md: 'auto' },
            width: '100%',
            pb: 0.5,
          }}
        >
          <Box sx={{ minWidth: { xs: 'auto', md: 960 } }}>
            {/* Top Row: Hosts Health */}
            <Grid
              container
              spacing={0}
              alignItems="stretch"
              sx={{ flexWrap: { xs: 'wrap', md: 'nowrap' } }}
            >
              <Grid
                item
                xs={12}
                sm={6}
                md={3}
                sx={{
                  ...METRICS_COLUMN_STYLE,
                  minWidth: { xs: 'auto', md: 220 },
                  borderRight: {
                    xs: 'none',
                    sm: BORDER_STYLE,
                    md: BORDER_STYLE,
                  },
                  borderBottom: {
                    xs: BORDER_STYLE,
                    sm: BORDER_STYLE,
                    md: 'none',
                  },
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
                      [FILTER_KEYS.HEALTH_CATEGORY]: [],
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
                  ...METRICS_COLUMN_STYLE,
                  minWidth: { xs: 'auto', md: 220 },
                  borderRight: {
                    xs: 'none',
                    sm: 'none',
                    md: BORDER_STYLE,
                  },
                  borderBottom: {
                    xs: BORDER_STYLE,
                    sm: BORDER_STYLE,
                    md: 'none',
                  },
                }}
              >
                <SingleMetric
                  name="Hosts Running"
                  value={hostsRunning}
                  total={totalHosts}
                  loading={isLoading}
                  Icon={<CheckIcon sx={{ color: colors.emerald }} />}
                  handleClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.LAB_RUNNING],
                      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
                      [FILTER_KEYS.HEALTH_CATEGORY]: [],
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
                  ...METRICS_COLUMN_STYLE,
                  minWidth: { xs: 'auto', md: 220 },
                  borderRight: {
                    xs: 'none',
                    sm: BORDER_STYLE,
                    md: BORDER_STYLE,
                  },
                  borderBottom: {
                    xs: BORDER_STYLE,
                    sm: 'none',
                    md: 'none',
                  },
                }}
              >
                <SingleMetric
                  name="Hosts Missing"
                  value={hostsMissing}
                  total={totalHosts}
                  loading={isLoading}
                  Icon={<ErrorIcon sx={{ color: colors.rose }} />}
                  handleClick={() =>
                    setFiltersBatch({
                      [FILTER_KEYS.STATE]: [androidState.LAB_MISSING],
                      [FILTER_KEYS.MACHINE_TYPE]: ['host'],
                      [FILTER_KEYS.HEALTH_CATEGORY]: [],
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
                  ...METRICS_COLUMN_STYLE,
                  minWidth: { xs: 'auto', md: 220 },
                  display: { xs: 'none', sm: 'block', md: 'block' },
                }}
              />
            </Grid>

            <Divider sx={{ my: 0.5, borderColor: 'rgba(0, 0, 0, 0.05)' }} />

            {/* Bottom Row: Devices Health */}
            <Grid
              container
              spacing={0}
              alignItems="stretch"
              sx={{ flexWrap: { xs: 'wrap', md: 'nowrap' } }}
            >
              {/* Total Devices Column */}
              <Grid
                item
                xs={12}
                sm={6}
                md={3}
                sx={{
                  ...METRICS_COLUMN_STYLE,
                  minWidth: { xs: 'auto', md: 220 },
                  borderRight: {
                    xs: 'none',
                    sm: BORDER_STYLE,
                    md: BORDER_STYLE,
                  },
                  borderBottom: {
                    xs: BORDER_STYLE,
                    sm: BORDER_STYLE,
                    md: 'none',
                  },
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
                      [FILTER_KEYS.HEALTH_CATEGORY]: [],
                    })
                  }
                />
                {showAvgUtilization && renderUtilizationSection(avg7d, avg30d)}
              </Grid>

              {/* Health Category Buckets */}
              {healthBuckets.map((bucketConfig, i) => (
                <Grid
                  key={bucketConfig.name}
                  item
                  xs={12}
                  sm={6}
                  md={3}
                  aria-label={`${bucketConfig.name} Devices`}
                  sx={{
                    ...METRICS_COLUMN_STYLE,
                    minWidth: { xs: 'auto', md: 220 },
                    borderRight: {
                      xs: 'none',
                      sm: (i + 1) % 2 === 0 ? BORDER_STYLE : 'none',
                      md: i < healthBuckets.length - 1 ? BORDER_STYLE : 'none',
                    },
                    borderBottom: {
                      xs: i < healthBuckets.length - 1 ? BORDER_STYLE : 'none',
                      sm: Math.floor((i + 1) / 2) < 1 ? BORDER_STYLE : 'none',
                      md: 'none',
                    },
                  }}
                >
                  <SingleMetric
                    name={bucketConfig.name}
                    value={bucketConfig.bucket?.total || 0}
                    total={totalDevices}
                    loading={isLoading}
                    Icon={bucketConfig.icon}
                    infoTooltip={
                      <InfoTooltip paperCss={{ maxWidth: '350px' }}>
                        {bucketConfig.tooltipText}
                      </InfoTooltip>
                    }
                    handleClick={() =>
                      setFiltersBatch({
                        [FILTER_KEYS.HEALTH_CATEGORY]:
                          bucketConfig.categoryKeys,
                        [FILTER_KEYS.MACHINE_TYPE]: ['device'],
                        [FILTER_KEYS.STATE]: [],
                      })
                    }
                  />
                  {showAllStates && (
                    <Box
                      sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        mt: 0.5,
                        mb: 2,
                        px: 0.5,
                        gap: 0.5,
                      }}
                    >
                      {renderStatusBreakdown(
                        bucketConfig.bucket,
                        bucketConfig.categoryKeys,
                      )}
                    </Box>
                  )}
                </Grid>
              ))}
            </Grid>
          </Box>
        </Box>
      </Box>
    </HoverMetricsContainer>
  );
}
