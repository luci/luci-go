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
  readonly enterDelay?: number;
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
  enterDelay,
}: SegmentedHealthBarProps) => {
  const safeBreakdown = breakdown ?? EMPTY_BREAKDOWN;
  const stats = useMemo(
    () => computeBreakdownStats(safeBreakdown),
    [safeBreakdown],
  );

  const hasData = stats.total > 0;
  const activeStates = stats.states.filter((s) => s.count > 0);

  const ariaDescription =
    activeStates.length > 0
      ? activeStates
          .map((s) => `${s.config.label}: ${s.count} (${s.pct}%)`)
          .join(', ')
      : 'No devices';

  return (
    <HealthStatePopover
      title={title}
      breakdown={safeBreakdown}
      availabilityPct={availabilityPct}
      healthPct={healthPct}
      baseFilter={baseFilter}
      enterDelay={enterDelay}
    >
      <Box
        data-testid={testId ?? `segmented-health-bar-${title}`}
        tabIndex={0}
        role="progressbar"
        aria-valuenow={availabilityPct ?? healthPct}
        aria-label={`${title} device health state distribution: ${ariaDescription}`}
        sx={{
          display: 'flex',
          alignItems: 'center',
          width: '100%',
          py: 0.5,
          boxSizing: 'border-box',
          cursor: hasData ? 'pointer' : 'default',
          position: 'relative',
          bgcolor: 'transparent',
          ...focusRingSx,
          ...(hasData && {
            '&:hover .segmented-bar-track, &:focus-visible .segmented-bar-track':
              {
                transform: 'scaleY(1.75)',
                filter: 'brightness(1.08)',
              },
          }),
        }}
      >
        <Box
          className="segmented-bar-track"
          sx={{
            display: 'flex',
            width: '100%',
            height,
            borderRadius: radii.pill,
            overflow: 'hidden',
            isolation: 'isolate',
            pointerEvents: 'none',
            bgcolor: 'grey.200',
            position: 'relative',
            transformOrigin: 'center',
            transition:
              'transform 0.15s cubic-bezier(0.4, 0, 0.2, 1), filter 0.15s ease',
          }}
        >
          {activeStates.map((s, index) => (
            <Box
              key={s.config.key}
              data-testid={`bar-segment-${s.config.key}`}
              sx={{
                width: `${s.pct}%`,
                flex: `${s.count} 0 0%`,
                bgcolor: s.config.color,
                height: '100%',
                transition: 'width 0.3s ease',
                ...(index === 0 && {
                  borderTopLeftRadius: 'inherit',
                  borderBottomLeftRadius: 'inherit',
                }),
                ...(index === activeStates.length - 1 && {
                  borderTopRightRadius: 'inherit',
                  borderBottomRightRadius: 'inherit',
                }),
              }}
            />
          ))}
        </Box>
      </Box>
    </HealthStatePopover>
  );
};
