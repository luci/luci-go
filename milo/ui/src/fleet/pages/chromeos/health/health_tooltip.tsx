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

import { Tooltip, TooltipProps } from '@mui/material';

import {
  COMPACT_TOOLTIP_PAPER_SX,
  INFO_TOOLTIP_PAPER_SX,
} from '@/fleet/components/info_tooltip/info_tooltip_styles';

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
