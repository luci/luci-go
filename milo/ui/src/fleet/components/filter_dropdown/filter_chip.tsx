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

import { Chip, ChipProps, SxProps, Theme } from '@mui/material';
import { forwardRef } from 'react';

export interface FilterChipProps extends ChipProps {
  /**
   * Optional maximum width for the chip before truncating with ellipsis.
   * Defaults to `'100%'` so chips never overflow their parent container.
   */
  readonly maxWidth?: number | string;
}

/**
 * Shared filter chip component used across both the interactive filter bar
 * (`SelectedChip`) and read-only filter displays (such as priority score
 * breakdowns). Enforces consistent `variant="filter"` styling and text
 * truncation with ellipsis so long filter expressions never overflow.
 */
export const FilterChip = forwardRef<HTMLDivElement, FilterChipProps>(
  function FilterChip(
    { maxWidth = '100%', variant = 'filter', sx, ...chipProps },
    ref,
  ) {
    const baseSx: SxProps<Theme> = {
      maxWidth,
      overflow: 'hidden',
      '& .MuiChip-label': {
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
      },
    };

    const mergedSx: SxProps<Theme> = sx
      ? [baseSx, ...(Array.isArray(sx) ? sx : [sx])]
      : baseSx;

    return <Chip ref={ref} variant={variant} sx={mergedSx} {...chipProps} />;
  },
);
