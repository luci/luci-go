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

import { PushPin } from '@mui/icons-material';
import { Box, Tooltip } from '@mui/material';

import { focusRingSx } from '@/fleet/theme/recipes';
import { fleetColors, fontSizes, radii } from '@/fleet/theme/tokens';

export interface RepairQueueRankCellProps {
  readonly rank: number;
  readonly isPinned: boolean;
}

/**
 * Renders the 1-based objective queue rank of a repair task, adding a pin
 * indicator and accessible tooltip when the row is pinned to the top for the
 * authenticated user.
 */
export const RepairQueueRankCell = ({
  rank,
  isPinned,
}: RepairQueueRankCellProps) => {
  if (!isPinned) {
    return (
      <Box component="span" sx={{ fontVariantNumeric: 'tabular-nums' }}>
        {rank}
      </Box>
    );
  }

  return (
    <Tooltip title="Pinned to top (assigned to you)" arrow>
      <Box
        component="span"
        tabIndex={0}
        aria-label={`Rank ${rank}, pinned to top`}
        sx={{
          ...focusRingSx,
          borderRadius: radii.sm,
          display: 'inline-flex',
          alignItems: 'center',
          gap: 0.5,
          color: fleetColors.primary,
          fontWeight: 600,
          fontVariantNumeric: 'tabular-nums',
          cursor: 'default',
        }}
      >
        <PushPin
          aria-hidden="true"
          sx={{
            fontSize: fontSizes.body,
            transform: 'rotate(45deg)',
            flexShrink: 0,
          }}
        />
        <span>{rank}</span>
      </Box>
    </Tooltip>
  );
};
