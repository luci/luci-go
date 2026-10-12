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

import { Box, Tooltip, TooltipProps, Typography } from '@mui/material';

import {
  COMPACT_TOOLTIP_PAPER_SX,
  INFO_TOOLTIP_PAPER_SX,
} from '@/fleet/components/info_tooltip/info_tooltip_styles';

import { getHealthStatus, HEALTH_THRESHOLDS } from './health_status_utils';

export interface HealthTooltipProps extends TooltipProps {
  /**
   * Layout density / padding variant:
   * - 'compact' (default): p: 1 (8px), max-width 320px for concise metrics, deltas, and SLA tables.
   * - 'standard': comfortable p: 2 (16px), max-width 450px for longer descriptions and multi-paragraph popovers.
   */
  readonly variant?: 'compact' | 'standard';
}

/**
 * Standard white hovercard tooltip for the ChromeOS Health Dashboard.
 * Pre-configures the light paper background, hairline divider border,
 * matching white arrow, and immediate 50ms enter delay.
 */
export const HealthTooltip = ({
  arrow = true,
  enterDelay = 50,
  variant = 'compact',
  slotProps,
  children,
  ...props
}: HealthTooltipProps) => {
  const paperSx =
    variant === 'standard' ? INFO_TOOLTIP_PAPER_SX : COMPACT_TOOLTIP_PAPER_SX;

  const userTooltipProps =
    typeof slotProps?.tooltip === 'object' && slotProps.tooltip !== null
      ? slotProps.tooltip
      : undefined;

  const mergedTooltipSx = userTooltipProps?.sx
    ? [
        paperSx,
        ...(Array.isArray(userTooltipProps.sx)
          ? userTooltipProps.sx
          : [userTooltipProps.sx]),
      ]
    : paperSx;

  return (
    <Tooltip
      arrow={arrow}
      enterDelay={enterDelay}
      slotProps={{
        ...slotProps,
        tooltip: {
          ...userTooltipProps,
          sx: mergedTooltipSx,
        },
      }}
      {...props}
    >
      {children}
    </Tooltip>
  );
};

export interface HealthThresholdsTooltipContentProps {
  /** Optional current percentage (0-100+) to display alongside the thresholds. */
  readonly pct?: number | undefined;
  /** Optional metric label, e.g. "Availability" or "Health". Defaults to "Availability". */
  readonly metricLabel?: string | undefined;
}

/**
 * Rich content for the health progress bar and status label tooltip:
 * displays optional current value header, followed by color-coded indicator dots,
 * SLA band labels (Healthy, Warning, Critical), and percentage ranges derived
 * from the single source of truth.
 */
export const HealthThresholdsTooltipContent = ({
  pct,
  metricLabel = 'Availability',
}: HealthThresholdsTooltipContentProps = {}) => {
  const currentStatus = pct !== undefined ? getHealthStatus(pct) : undefined;

  return (
    <Box
      sx={{ p: 0.25, minWidth: 175 }}
      data-testid="health-thresholds-tooltip"
    >
      {pct !== undefined && currentStatus !== undefined && (
        <Box
          data-testid="threshold-current-value"
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 1.5,
            pb: 0.5,
            mb: 0.5,
            borderBottom: '1px solid',
            borderColor: 'divider',
          }}
        >
          <Typography
            variant="caption"
            sx={{ fontWeight: 500, color: 'text.secondary' }}
          >
            Current {metricLabel}:
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Box
              sx={{
                width: 7,
                height: 7,
                borderRadius: '50%',
                bgcolor: `${currentStatus.color}.main`,
              }}
            />
            <Typography
              variant="caption"
              sx={{ fontWeight: 600, color: `${currentStatus.color}.main` }}
            >
              {pct}% ({currentStatus.status})
            </Typography>
          </Box>
        </Box>
      )}

      <Typography
        variant="caption"
        sx={{
          fontWeight: 600,
          color: 'text.secondary',
          display: 'block',
          mb: 0.5,
        }}
      >
        Health Thresholds
      </Typography>

      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.35 }}>
        {HEALTH_THRESHOLDS.map((tier) => {
          const isCurrentTier = currentStatus?.status === tier.status;
          return (
            <Box
              key={tier.status}
              data-testid={`threshold-row-${tier.status.toLowerCase()}`}
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 1.5,
                px: isCurrentTier ? 0.5 : 0,
                py: isCurrentTier ? 0.2 : 0,
                borderRadius: 0.75,
                bgcolor: isCurrentTier ? 'action.selected' : 'transparent',
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
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
                    fontWeight: isCurrentTier ? 600 : 400,
                    color: 'text.primary',
                  }}
                >
                  {tier.label}
                  {isCurrentTier && ' (Current)'}
                </Typography>
              </Box>
              <Typography
                variant="caption"
                sx={{
                  fontWeight: 400,
                  color: isCurrentTier ? 'text.primary' : 'text.secondary',
                  flexShrink: 0,
                }}
              >
                {tier.range}
              </Typography>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
};

export interface HealthThresholdsTooltipProps
  extends Omit<HealthTooltipProps, 'title'> {
  /** Optional current percentage (0-100+) to display alongside the thresholds. */
  readonly pct?: number | undefined;
  /** Optional metric label, e.g. "Availability" or "Health". Defaults to "Availability". */
  readonly metricLabel?: string | undefined;
}

/**
 * Convenient wrapper rendering a HealthTooltip pre-loaded with SLA thresholds content.
 */
export const HealthThresholdsTooltip = ({
  pct,
  metricLabel,
  children,
  ...tooltipProps
}: HealthThresholdsTooltipProps) => (
  <HealthTooltip
    title={
      <HealthThresholdsTooltipContent pct={pct} metricLabel={metricLabel} />
    }
    {...tooltipProps}
  >
    {children}
  </HealthTooltip>
);
