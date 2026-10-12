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

import { SxProps, Theme } from '@mui/material';

/**
 * Styling for the arrow of light white hovercard surfaces.
 * Matches the background.paper color and divider border of the tooltip box.
 */
export const WHITE_TOOLTIP_ARROW_SX = {
  color: 'background.paper',
  '&::before': {
    border: (theme: Theme) => `1px solid ${theme.palette.divider}`,
  },
} as const satisfies SxProps<Theme>;

/**
 * Common base styling for the light "info bauble" / hovercard surface:
 * a white paper background with dark text, hairline border, subtle drop shadow,
 * rounded corners, and matching white arrow.
 */
export const BASE_WHITE_TOOLTIP_PAPER_SX = {
  bgcolor: 'background.paper',
  color: 'text.primary',
  border: (theme: Theme) => `1px solid ${theme.palette.divider}`,
  borderRadius: 1.5,
  boxShadow: 3,
  '& .MuiTooltip-arrow': WHITE_TOOLTIP_ARROW_SX,
} as const satisfies SxProps<Theme>;

/**
 * Standard hovercard tooltip styling with comfortable padding (p: 2)
 * for detailed explanatory popovers, such as InfoTooltip, repair queue priority
 * score breakdowns, and Quota vs. Enrolled baseline descriptions.
 */
export const INFO_TOOLTIP_PAPER_SX = {
  ...BASE_WHITE_TOOLTIP_PAPER_SX,
  p: 2,
  maxWidth: 450,
  pointerEvents: 'auto',
  fontSize: '0.875rem',
} as const satisfies SxProps<Theme>;

export const INFO_TOOLTIP_SLOT_PROPS = {
  tooltip: {
    sx: INFO_TOOLTIP_PAPER_SX,
  },
} as const;

/**
 * Compact hovercard tooltip styling with tighter padding (p: 1)
 * and smaller max-width (320px) for concise metrics, SLA thresholds,
 * delta comparisons, and data points.
 */
export const COMPACT_TOOLTIP_PAPER_SX = {
  ...BASE_WHITE_TOOLTIP_PAPER_SX,
  p: 1,
  maxWidth: 320,
} as const satisfies SxProps<Theme>;

export const COMPACT_TOOLTIP_SLOT_PROPS = {
  tooltip: {
    sx: COMPACT_TOOLTIP_PAPER_SX,
  },
} as const;

/**
 * Styling for custom MUI `<Paper>` elements used in Recharts chart tooltips
 * (e.g. historical availability trends), matching the compact white hovercard
 * treatment.
 */
export const TRENDS_TOOLTIP_PAPER_SX = {
  bgcolor: 'background.paper',
  color: 'text.primary',
  p: 1,
  border: (theme: Theme) => `1px solid ${theme.palette.divider}`,
  borderRadius: 1.5,
} as const satisfies SxProps<Theme>;
