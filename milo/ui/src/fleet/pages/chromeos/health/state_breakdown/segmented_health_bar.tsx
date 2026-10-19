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

import { Box, Popover } from '@mui/material';
import { useMemo, useRef, useState } from 'react';

import { focusRingSx } from '@/fleet/theme/recipes';
import { radii } from '@/fleet/theme/tokens';

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
} from './compute_breakdown_stats';
import { HealthStatePopoverContent } from './health_state_popover_content';

export interface SegmentedHealthBarProps {
  readonly title: string;
  readonly breakdown: DeviceStateBreakdown;
  readonly height?: number;
  readonly expectedCount?: number;
  readonly targetAvailabilityPct?: number;
  readonly availabilityPct?: number;
  readonly baseFilter?: string;
  readonly testId?: string;
}

export const SegmentedHealthBar = ({
  title,
  breakdown,
  height = 4,
  expectedCount,
  targetAvailabilityPct,
  availabilityPct,
  baseFilter,
  testId,
}: SegmentedHealthBarProps) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const barRef = useRef<HTMLDivElement | null>(null);

  const stats = useMemo(() => computeBreakdownStats(breakdown), [breakdown]);

  const handleMouseEnter = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMouseLeave = () => {
    setAnchorEl(null);
  };

  const open = Boolean(anchorEl);

  const ariaDescription = stats.states
    .filter((s) => s.count > 0)
    .map((s) => `${s.config.label}: ${s.count} (${s.pct}%)`)
    .join(', ');

  return (
    <>
      <Box
        ref={barRef}
        data-testid={testId ?? `segmented-health-bar-${title}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        onClick={(e) => {
          e.stopPropagation();
          setAnchorEl(e.currentTarget);
        }}
        tabIndex={0}
        role="progressbar"
        aria-label={`${title} device health state distribution: ${ariaDescription}`}
        sx={{
          display: 'flex',
          width: '100%',
          height,
          borderRadius: radii.sm,
          overflow: 'hidden',
          bgcolor: 'grey.200',
          cursor: 'pointer',
          position: 'relative',
          transition: 'all 0.15s ease-in-out',
          '&:hover': {
            transform: 'scaleY(1.5)',
            filter: 'brightness(1.08)',
          },
          ...focusRingSx,
        }}
      >
        {stats.states
          .filter((s) => s.count > 0)
          .map((s) => (
            <Box
              key={s.config.key}
              data-testid={`bar-segment-${s.config.key}`}
              sx={{
                width: `${s.pct}%`,
                bgcolor: s.config.color,
                height: '100%',
                transition: 'width 0.3s ease',
              }}
            />
          ))}
      </Box>

      {/* Interactive State Popover */}
      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={handleMouseLeave}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'center',
        }}
        transformOrigin={{
          vertical: 'bottom',
          horizontal: 'center',
        }}
        disableRestoreFocus
        slotProps={{
          paper: {
            onMouseEnter: () => setAnchorEl(anchorEl),
            onMouseLeave: handleMouseLeave,
            sx: {
              pointerEvents: 'auto',
              borderRadius: radii.md,
              mt: -1,
            },
          },
        }}
      >
        <HealthStatePopoverContent
          title={title}
          breakdown={breakdown}
          expectedCount={expectedCount}
          targetAvailabilityPct={targetAvailabilityPct}
          availabilityPct={availabilityPct}
          baseFilter={baseFilter}
        />
      </Popover>
    </>
  );
};
