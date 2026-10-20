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

import { Box } from '@mui/material';
import { useMemo } from 'react';

import { focusRingSx } from '@/fleet/theme/recipes';
import { radii } from '@/fleet/theme/tokens';

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
} from './compute_breakdown_stats';
import { HealthStatePopover } from './health_state_popover';

export interface SegmentedHealthBarProps {
  readonly title: string;
  readonly breakdown?: DeviceStateBreakdown | undefined;
  readonly height?: number;
  readonly availabilityPct?: number;
  readonly healthPct?: number;
  readonly baseFilter?: string;
  readonly testId?: string;
}

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

export const SegmentedHealthBar = ({
  title,
  breakdown,
  height = 4,
  availabilityPct,
  healthPct,
  baseFilter,
  testId,
}: SegmentedHealthBarProps) => {
  const safeBreakdown = breakdown ?? EMPTY_BREAKDOWN;
  const stats = useMemo(
    () => computeBreakdownStats(safeBreakdown),
    [safeBreakdown],
  );
  const hasData = stats.total > 0;

  const ariaDescription = stats.states
    .filter((s) => s.count > 0)
    .map((s) => `${s.config.label}: ${s.count} (${s.pct}%)`)
    .join(', ');

  return (
    <HealthStatePopover
      title={title}
      breakdown={safeBreakdown}
      availabilityPct={availabilityPct}
      healthPct={healthPct}
      baseFilter={baseFilter}
    >
      <Box
        data-testid={testId ?? `segmented-health-bar-${title}`}
        tabIndex={0}
        role="progressbar"
        aria-label={`${title} device health state distribution: ${ariaDescription || 'No devices'}`}
        sx={{
          display: 'flex',
          width: '100%',
          height,
          borderRadius: radii.sm,
          overflow: 'hidden',
          bgcolor: 'grey.200',
          cursor: hasData ? 'pointer' : 'default',
          position: 'relative',
          transition: 'all 0.15s ease-in-out',
          ...(hasData && {
            '&:hover': {
              transform: 'scaleY(1.5)',
              filter: 'brightness(1.08)',
            },
          }),
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
    </HealthStatePopover>
  );
};
