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

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
  formatStateCount,
} from './compute_breakdown_stats';
import { ExactDutStateKey } from './health_state_definitions';
import { getDeviceListStateUrl } from './state_navigation_utils';

export interface HealthStatePopoverContentProps {
  readonly title: string;
  readonly breakdown: DeviceStateBreakdown;
  readonly expectedCount?: number;
  readonly targetAvailabilityPct?: number;
  readonly availabilityPct?: number;
  readonly baseFilter?: string;
}

export const HealthStatePopoverContent = ({
  title,
  breakdown,
  expectedCount,
  targetAvailabilityPct,
  availabilityPct: explicitAvailabilityPct,
  baseFilter,
}: HealthStatePopoverContentProps) => {
  const stats = useMemo(() => computeBreakdownStats(breakdown), [breakdown]);

  const total = stats.total;
  const healthyCount = stats.healthyTotalCount;
  const healthPct = stats.healthyTotalPct;

  // Availability and Health are computed from the EXACT same set of healthy devices
  // (ready + needs_repair + repair_failed + reserved).
  // The ONLY difference in Availability is the denominator: expected count (quota target)
  // instead of total enrolled devices in the fleet.
  const displayAvailabilityPct = useMemo(() => {
    if (explicitAvailabilityPct !== undefined) {
      return explicitAvailabilityPct;
    }
    if (expectedCount !== undefined && expectedCount > 0) {
      return Math.round((healthyCount / expectedCount) * 1000) / 10;
    }
    return healthPct;
  }, [explicitAvailabilityPct, expectedCount, healthyCount, healthPct]);

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
                color:
                  displayAvailabilityPct >= 95
                    ? 'success.main'
                    : displayAvailabilityPct >= 80
                      ? 'warning.main'
                      : 'error.main',
              }}
            >
              {displayAvailabilityPct}%
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
            {expectedCount !== undefined && expectedCount > 0 && (
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  fontSize: 9.5,
                  color: 'text.secondary',
                  lineHeight: 1,
                  mt: 0.25,
                }}
              >
                {formatStateCount(healthyCount)} /{' '}
                {formatStateCount(expectedCount)} target
              </Typography>
            )}
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
                color:
                  healthPct >= 95
                    ? 'success.main'
                    : healthPct >= 80
                      ? 'warning.main'
                      : 'error.main',
              }}
            >
              {healthPct}%
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
            <Typography
              variant="caption"
              sx={{
                display: 'block',
                fontSize: 9.5,
                color: 'text.secondary',
                lineHeight: 1,
                mt: 0.25,
              }}
            >
              {formatStateCount(healthyCount)} / {formatStateCount(total)} fleet
            </Typography>
          </Box>
        </Box>
      </Box>

      {/* Target Baseline Comparison if provided */}
      {targetAvailabilityPct !== undefined && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            bgcolor: 'action.hover',
            p: 0.75,
            px: 1.25,
            borderRadius: radii.sm,
            mb: 1.5,
          }}
        >
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            Expected Quota Target
          </Typography>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color:
                displayAvailabilityPct >= targetAvailabilityPct
                  ? 'success.main'
                  : 'error.main',
            }}
          >
            {targetAvailabilityPct}% (
            {displayAvailabilityPct >= targetAvailabilityPct
              ? 'Met'
              : `${Math.round(targetAvailabilityPct - displayAvailabilityPct)}% deficit`}
            )
          </Typography>
        </Box>
      )}

      {/* Mini Color Spectrum Bar */}
      <Box
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

      {/* Detailed Categorized States */}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
        {stats.categories.map((cat) => (
          <Box key={cat.category}>
            {/* Category Header */}
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                mb: 0.5,
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
            <Box
              sx={{
                bgcolor: 'background.paper',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: radii.sm,
                overflow: 'hidden',
              }}
            >
              {cat.states.map((st, idx) => (
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
                      idx < cat.states.length - 1 ? '1px solid' : 'none',
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

                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
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
          </Box>
        ))}
      </Box>

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
