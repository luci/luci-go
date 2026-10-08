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

import { HelpOutline } from '@mui/icons-material';
import {
  Box,
  Card,
  CardContent,
  LinearProgress,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo } from 'react';

import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';
import {
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { BaselineChip } from './baseline_chip';
import { getHealthStatus } from './health_status_utils';
import {
  getHourAlignedTimeWindow,
  RECENT_HOURS_WINDOW,
} from './time_window_utils';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

export interface HeroAvailabilityCardProps {
  /** Optional active AIP-160 filter expression. */
  readonly filter?: string;
}

export const HeroAvailabilityCard = ({
  filter = '',
}: HeroAvailabilityCardProps) => {
  const queryRequest = useMemo(() => {
    const { startTime, endTime } =
      getHourAlignedTimeWindow(RECENT_HOURS_WINDOW);
    return {
      grouping: TrendlineGrouping.GROUP_BY_OVERALL,
      startTime,
      endTime,
      filter,
    };
  }, [filter]);

  const { data: trendsData, isLoading: isTrendsLoading } =
    useFleetAvailabilityTrends(queryRequest);

  const points = useMemo(
    () => trendsData?.series?.[0]?.points ?? [],
    [trendsData?.series],
  );

  // Take the freshest point.
  const activePoint = useMemo(
    () =>
      points.length > 0
        ? points.reduce((freshest, point) =>
            (point.timestamp ?? '') > (freshest.timestamp ?? '')
              ? point
              : freshest,
          )
        : undefined,
    [points],
  );

  // Value from pRPC is a ratio (e.g. 0.95 = 95%, 1.05 = 105% when surplus devices exceed expected quota target).
  const pct = useMemo(() => {
    if (!activePoint) return undefined;
    return Math.max(0, Math.round(activePoint.value * 100));
  }, [activePoint]);

  const isAvailability =
    trendsData?.metricType === TrendlineMetricType.AVAILABILITY;

  const metricLabel = isAvailability
    ? 'Current Fleet Availability'
    : 'Current Filtered Health';

  const healthMeta = pct !== undefined ? getHealthStatus(pct) : undefined;

  return (
    <Card
      variant="outlined"
      data-testid="hero-availability-card"
      sx={{
        mb: 4,
        bgcolor: 'background.paper',
        boxShadow: '0 1px 3px rgba(0,0,0,0.05)',
        borderRadius: 2,
      }}
    >
      <CardContent sx={{ p: 3 }}>
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: 2,
          }}
        >
          {/* Left Column: Metric Label, Baseline Chip, Help Tooltip, Big Availability % */}
          <Box>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
              }}
            >
              <Typography
                variant="overline"
                color="text.secondary"
                sx={{ fontWeight: 'bold', letterSpacing: 0.5 }}
              >
                {metricLabel}
              </Typography>
              <BaselineChip
                isAvailability={isAvailability}
                testId="hero-baseline-chip"
              />
              <Tooltip
                title={
                  <Box sx={{ p: 0.5 }}>
                    <Typography
                      variant="caption"
                      display="block"
                      sx={{ fontWeight: 'bold', mb: 0.5 }}
                    >
                      Availability vs. Health:
                    </Typography>
                    <Typography
                      variant="caption"
                      display="block"
                      sx={{ mb: 0.5 }}
                    >
                      • <b>Availability (Quota Baseline):</b> % of devices
                      available out of expected quota target configured per
                      model.
                    </Typography>
                    <Typography
                      variant="caption"
                      display="block"
                      sx={{ mb: 0.5 }}
                    >
                      • <b>Health (Enrolled Baseline):</b> Expected quotas are
                      configured per model overall. When filtered by pool, zone,
                      or arbitrary labels, baseline falls back to actual
                      enrolled physical DUTs.
                    </Typography>
                    <Typography
                      variant="caption"
                      display="block"
                      color="text.secondary"
                      sx={{ fontStyle: 'italic', mt: 0.5 }}
                    >
                      Note: Device availability is currently calculated based on
                      devices in <b>dut_state = READY</b>, which may differ from
                      other dashboards.
                    </Typography>
                  </Box>
                }
                arrow
                slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
              >
                <HelpOutline
                  data-testid="hero-help-icon"
                  sx={{
                    fontSize: 14,
                    color: 'text.secondary',
                    cursor: 'pointer',
                  }}
                />
              </Tooltip>
            </Box>

            <Box
              sx={{
                display: 'flex',
                alignItems: 'baseline',
                gap: 1,
                mt: 0.5,
              }}
            >
              <Typography
                variant="h3"
                data-testid="hero-availability-percentage"
                sx={{
                  fontWeight: 'bold',
                  color:
                    healthMeta !== undefined
                      ? `${healthMeta.color}.main`
                      : 'text.primary',
                }}
              >
                {pct !== undefined ? `${pct}%` : isTrendsLoading ? '...' : '—'}
              </Typography>
            </Box>
          </Box>

          {/* Right Column: Fleet Availability / Health progress bar and SLA status */}
          <Box
            sx={{
              display: 'flex',
              flexDirection: { xs: 'column', md: 'row' },
              alignItems: 'center',
              gap: 3,
              width: { xs: '100%', sm: 260 },
            }}
          >
            <Box sx={{ width: '100%' }}>
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  mb: 0.5,
                }}
              >
                <Typography variant="body2" color="text.secondary">
                  {isAvailability ? 'Fleet Availability' : 'Fleet Health'}
                </Typography>
                <Typography
                  variant="body2"
                  data-testid="hero-status-label"
                  sx={{ fontWeight: 'bold' }}
                  color={
                    healthMeta !== undefined
                      ? `${healthMeta.color}.main`
                      : 'text.secondary'
                  }
                >
                  {healthMeta !== undefined
                    ? `${healthMeta.status} (${pct}%)`
                    : isTrendsLoading
                      ? 'Loading...'
                      : 'No data'}
                </Typography>
              </Box>
              <LinearProgress
                data-testid="hero-availability-progress"
                variant={isTrendsLoading ? 'indeterminate' : 'determinate'}
                value={Math.min(100, pct ?? 0)}
                color={healthMeta?.color ?? 'primary'}
                sx={{ height: 8, borderRadius: 4 }}
              />
            </Box>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
};
