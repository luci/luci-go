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

import { OpenInNew } from '@mui/icons-material';
import { Box, Button, Divider, Paper, Typography } from '@mui/material';
import { useMemo } from 'react';

import { healthToDeviceFilter } from '@/fleet/adapters/filter_adapters';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { radii } from '@/fleet/theme/tokens';

import { getHealthStatus, HEALTH_THRESHOLDS } from '../health_status_utils';

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
  formatStateCount,
} from './compute_breakdown_stats';
import { ExactDutStateKey } from './health_state_definitions';
import { getDeviceListStateUrl } from './state_navigation_utils';

const EMPTY_BREAKDOWN: DeviceStateBreakdown = {
  total: 0,
  ready: 0,
  needsRepair: 0,
  repairFailed: 0,
  needsManualRepair: 0,
  needsReplacement: 0,
  reserved: 0,
  needsDeploy: 0,
};

export interface HealthStatePopoverContentProps {
  readonly title: string;
  readonly breakdown?: DeviceStateBreakdown | undefined;
  readonly availabilityPct?: number;
  readonly healthPct?: number;
  readonly baseFilter?: string;
}

export const HealthStatePopoverContent = ({
  title,
  breakdown = EMPTY_BREAKDOWN,
  availabilityPct: explicitAvailabilityPct,
  healthPct: explicitHealthPct,
  baseFilter,
}: HealthStatePopoverContentProps) => {
  const stats = useMemo(() => computeBreakdownStats(breakdown), [breakdown]);

  const total = stats.total;
  const healthPct = explicitHealthPct ?? stats.healthyTotalPct;

  // Availability and Health:
  // If explicit availabilityPct is provided (e.g. from SLA/Quota BigQuery trendline point),
  // use that; otherwise fall back to fleet-wide health percentage.
  const displayAvailabilityPct = useMemo(() => {
    if (explicitAvailabilityPct !== undefined) {
      return explicitAvailabilityPct;
    }
    return healthPct;
  }, [explicitAvailabilityPct, healthPct]);

  const availabilityStatus = getHealthStatus(displayAvailabilityPct);
  const healthStatus = getHealthStatus(healthPct);

  const adaptedBaseFilter = useMemo(
    () => healthToDeviceFilter(baseFilter),
    [baseFilter],
  );

  const allDevicesUrl = `/ui/fleet/p/chromeos/devices${
    adaptedBaseFilter
      ? `?${FILTERS_PARAM_KEY}=${encodeURIComponent(adaptedBaseFilter)}`
      : ''
  }`;

  return (
    <Paper
      elevation={4}
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
      sx={{
        p: 2,
        width: 380,
        maxWidth: '92vw',
        borderRadius: radii.md,
        bgcolor: 'background.paper',
        border: '1px solid',
        borderColor: 'divider',
      }}
    >
      {/* Header with Title and Dual Metrics (Availability vs Health) */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          mb: 1.5,
          gap: 1,
        }}
      >
        <Box sx={{ minWidth: 0 }}>
          <Typography
            variant="subtitle1"
            sx={{ fontWeight: 700, lineHeight: 1.2, color: 'text.primary' }}
          >
            {title}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {formatStateCount(total)} total enrolled
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'flex-start' }}>
          <Box sx={{ textAlign: 'right' }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 800,
                lineHeight: 1,
                color: `${availabilityStatus.color}.main`,
              }}
            >
              {displayAvailabilityPct.toFixed(1)}%
            </Typography>
            <Typography
              variant="caption"
              sx={{
                fontSize: 10,
                color: 'text.secondary',
                fontWeight: 600,
                display: 'block',
                mt: 0.25,
              }}
            >
              Availability
            </Typography>
          </Box>
          <Divider
            orientation="vertical"
            flexItem
            sx={{ alignSelf: 'stretch', my: 0.25 }}
          />
          <Box sx={{ textAlign: 'right' }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 800,
                lineHeight: 1,
                color: `${healthStatus.color}.main`,
              }}
            >
              {healthPct.toFixed(1)}%
            </Typography>
            <Typography
              variant="caption"
              sx={{
                fontSize: 10,
                color: 'text.secondary',
                fontWeight: 600,
                display: 'block',
                mt: 0.25,
              }}
            >
              Health
            </Typography>
          </Box>
        </Box>
      </Box>

      {/* SLA Health Thresholds Strip */}
      <Box
        data-testid="popover-thresholds-strip"
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 1,
          py: 0.5,
          mb: 1.5,
          borderRadius: radii.sm,
          bgcolor: 'action.hover',
          border: '1px solid',
          borderColor: 'divider',
        }}
      >
        <Typography
          variant="caption"
          sx={{
            fontSize: 10,
            fontWeight: 700,
            textTransform: 'uppercase',
            letterSpacing: 0.5,
            color: 'text.secondary',
          }}
        >
          SLA
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {HEALTH_THRESHOLDS.map((tier) => {
            const isCurrentTier = availabilityStatus.status === tier.status;
            return (
              <Box
                key={tier.status}
                data-testid={`popover-threshold-${tier.status.toLowerCase()}`}
                title={`${tier.label}: ${tier.range}${isCurrentTier ? ' (Current tier)' : ''}`}
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.5,
                  px: isCurrentTier ? 0.75 : 0.25,
                  py: 0.2,
                  borderRadius: radii.sm,
                  bgcolor: isCurrentTier ? 'action.selected' : 'transparent',
                  border: '1px solid',
                  borderColor: isCurrentTier ? 'divider' : 'transparent',
                }}
              >
                <Box
                  sx={{
                    width: 7,
                    height: 7,
                    borderRadius: '50%',
                    bgcolor: tier.colorSx,
                    flexShrink: 0,
                  }}
                />
                <Typography
                  variant="caption"
                  sx={{
                    fontSize: 10.5,
                    fontWeight: isCurrentTier ? 700 : 500,
                    color: isCurrentTier ? 'text.primary' : 'text.secondary',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {tier.label} {tier.range}
                </Typography>
              </Box>
            );
          })}
        </Box>
      </Box>

      {/* Mini Color Spectrum Bar */}
      <Box
        data-testid="popover-spectrum-bar"
        sx={{
          display: 'flex',
          height: 6,
          borderRadius: radii.pill,
          overflow: 'hidden',
          width: '100%',
          bgcolor: 'grey.200',
          mb: 2,
        }}
      >
        {stats.states
          .filter((s) => s.count > 0)
          .map((s) => (
            <Box
              key={s.config.key}
              title={`${s.config.label}: ${s.count} (${s.pct}%)`}
              sx={{
                width: `${s.pct}%`,
                bgcolor: s.config.color,
                height: '100%',
              }}
            />
          ))}
      </Box>

      {/* Detailed Categorized States or Empty State */}
      {total === 0 ? (
        <Box
          data-testid="popover-empty-state"
          sx={{
            py: 2.5,
            px: 2,
            textAlign: 'center',
            bgcolor: 'action.hover',
            borderRadius: radii.sm,
            mt: 0.5,
          }}
        >
          <Typography
            variant="body2"
            sx={{ fontWeight: 600, color: 'text.secondary' }}
          >
            No devices enrolled
          </Typography>
          <Typography
            variant="caption"
            sx={{ color: 'text.disabled', display: 'block', mt: 0.5 }}
          >
            There are no devices registered in this cohort.
          </Typography>
        </Box>
      ) : (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          {stats.categories.map((cat) => {
            const activeStates = cat.states.filter((st) => st.count > 0);
            return (
              <Box key={cat.category}>
                {/* Category Header */}
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    mb: activeStates.length > 0 ? 0.5 : 0,
                  }}
                >
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 700,
                      textTransform: 'uppercase',
                      fontSize: 10.5,
                      letterSpacing: 0.5,
                      color: cat.color,
                    }}
                  >
                    {cat.label}
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 700,
                      color: 'text.secondary',
                      fontSize: 11,
                    }}
                  >
                    {formatStateCount(cat.count)} ({cat.pct}%)
                  </Typography>
                </Box>

                {/* Exact State Rows with 1-click AIP-160 drilldown */}
                {activeStates.length > 0 && (
                  <Box
                    sx={{
                      bgcolor: 'background.paper',
                      border: '1px solid',
                      borderColor: 'divider',
                      borderRadius: radii.sm,
                      overflow: 'hidden',
                    }}
                  >
                    {activeStates.map((st, idx) => (
                      <Box
                        key={st.config.key}
                        component="a"
                        href={getDeviceListStateUrl(
                          st.config.key as ExactDutStateKey,
                          baseFilter,
                        )}
                        target="_blank"
                        rel="noreferrer"
                        data-testid={`state-row-${st.config.key}`}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          py: 0.6,
                          px: 1.25,
                          textDecoration: 'none',
                          color: 'inherit',
                          borderBottom:
                            idx < activeStates.length - 1
                              ? '1px solid'
                              : 'none',
                          borderColor: 'divider',
                          transition: 'background-color 0.15s ease',
                          '&:hover': {
                            bgcolor: 'action.hover',
                            '& .state-link-icon': {
                              opacity: 1,
                              color: 'primary.main',
                            },
                          },
                        }}
                      >
                        <Box
                          sx={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 1,
                            minWidth: 0,
                          }}
                        >
                          <Box
                            sx={{
                              width: 8,
                              height: 8,
                              borderRadius: '50%',
                              bgcolor: st.config.color,
                              flexShrink: 0,
                            }}
                          />
                          <Typography
                            variant="body2"
                            sx={{
                              fontSize: 12,
                              fontWeight: 500,
                              color: 'text.primary',
                            }}
                          >
                            {st.config.label}
                          </Typography>
                        </Box>

                        <Box
                          sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
                        >
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 700,
                              fontSize: 12,
                              color: 'text.primary',
                            }}
                          >
                            {formatStateCount(st.count)}
                          </Typography>
                          <Typography
                            variant="caption"
                            sx={{
                              fontSize: 11,
                              color: 'text.secondary',
                              width: 42,
                              textAlign: 'right',
                            }}
                          >
                            {st.pct}%
                          </Typography>
                          <OpenInNew
                            className="state-link-icon"
                            sx={{
                              fontSize: 12,
                              opacity: 0.35,
                              transition: 'all 0.15s ease',
                            }}
                          />
                        </Box>
                      </Box>
                    ))}
                  </Box>
                )}
              </Box>
            );
          })}
        </Box>
      )}

      {/* Footer Quick Action */}
      <Divider sx={{ my: 1.5 }} />
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Typography variant="caption" color="text.secondary">
          Click any state to filter device list
        </Typography>
        <Button
          size="small"
          variant="text"
          component="a"
          href={allDevicesUrl}
          target="_blank"
          rel="noreferrer"
          sx={{
            textTransform: 'none',
            fontSize: 11,
            fontWeight: 600,
            p: 0.2,
          }}
        >
          View all devices
        </Button>
      </Box>
    </Paper>
  );
};
