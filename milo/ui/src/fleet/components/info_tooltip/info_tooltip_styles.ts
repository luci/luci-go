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
 * Styling for the light "info bauble" hovercard surface: a white paper
 * background with dark text, a hairline border and a drop shadow.
 *
 * Shared so that tooltips outside `InfoTooltip` itself — such as the repair
 * queue priority score breakdown — can adopt the same look without the two
 * treatments drifting apart.
 */
export const INFO_TOOLTIP_PAPER_SX = {
  bgcolor: 'background.paper',
  color: 'text.primary',
  p: 2,
  border: (theme: Theme) => `1px solid ${theme.palette.divider}`,
  boxShadow: 3,
  maxWidth: 450,
  pointerEvents: 'auto',
  fontSize: '0.875rem',
} as const satisfies SxProps<Theme>;
