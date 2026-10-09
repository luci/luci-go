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

import { Chip, Tooltip, type SxProps, type Theme } from '@mui/material';
import React from 'react';

import { INFO_TOOLTIP_PAPER_SX } from '@/fleet/components/info_tooltip/info_tooltip_styles';

export interface HealthChipProps {
  /** The text label displayed inside the chip. */
  readonly label: string;
  /** Optional tooltip content explaining the chip's meaning. */
  readonly tooltip?: React.ReactNode;
  /** Color variant: 'default' (outlined gray/divider) or 'primary' (outlined blue). */
  readonly color?: 'default' | 'primary';
  /** Optional custom testId for testing assertions. */
  readonly testId?: string;
  /** Custom style overrides. */
  readonly sx?: SxProps<Theme>;
}

/**
 * Standardized outlined metadata chip for the ChromeOS Health Dashboard.
 * Ensures consistent typography, height, borders, and tooltip behavior
 * across time-window tags, baseline indicators, and metric chips.
 */
export const HealthChip = ({
  label,
  tooltip,
  color = 'default',
  testId,
  sx,
}: HealthChipProps) => {
  const isPrimary = color === 'primary';

  const chip = (
    <Chip
      data-testid={testId}
      label={label}
      size="small"
      variant="outlined"
      sx={{
        height: 22,
        fontSize: 11,
        fontWeight: 600,
        color: isPrimary ? 'primary.main' : 'text.secondary',
        borderColor: isPrimary ? 'primary.main' : 'divider',
        cursor: tooltip ? 'help' : 'default',
        '& .MuiChip-label': {
          px: 1,
        },
        ...sx,
      }}
    />
  );

  if (!tooltip) {
    return chip;
  }

  return (
    <Tooltip
      title={tooltip}
      arrow
      slotProps={{ tooltip: { sx: INFO_TOOLTIP_PAPER_SX } }}
    >
      {chip}
    </Tooltip>
  );
};
