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

import { Popover, PopoverOrigin } from '@mui/material';
import { cloneElement, useEffect, useMemo, useRef, useState } from 'react';

import { radii } from '@/fleet/theme/tokens';

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
} from './compute_breakdown_stats';
import { HealthStatePopoverContent } from './health_state_popover_content';

export interface HealthStatePopoverProps {
  readonly title: string;
  readonly breakdown?: DeviceStateBreakdown | undefined;
  readonly availabilityPct?: number | undefined;
  readonly healthPct?: number | undefined;
  readonly baseFilter?: string | undefined;
  readonly disabled?: boolean;
  readonly anchorOrigin?: PopoverOrigin;
  readonly transformOrigin?: PopoverOrigin;
  readonly transitionDuration?: number | { enter?: number; exit?: number };
  readonly children: React.ReactElement<React.HTMLAttributes<HTMLElement>>;
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

const DEFAULT_ANCHOR_ORIGIN: PopoverOrigin = {
  vertical: 'top',
  horizontal: 'center',
};

const DEFAULT_TRANSFORM_ORIGIN: PopoverOrigin = {
  vertical: 'bottom',
  horizontal: 'center',
};

/**
 * Reusable hovercard popover wrapper that displays the complete DUT state breakdown
 * and horizontal SLA threshold status for any trigger element (e.g. status labels or progress bars).
 */
export const HealthStatePopover = ({
  title,
  breakdown,
  availabilityPct,
  healthPct,
  baseFilter,
  disabled = false,
  anchorOrigin = DEFAULT_ANCHOR_ORIGIN,
  transformOrigin = DEFAULT_TRANSFORM_ORIGIN,
  transitionDuration,
  children,
}: HealthStatePopoverProps) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const safeBreakdown = breakdown ?? EMPTY_BREAKDOWN;
  const stats = useMemo(
    () => computeBreakdownStats(safeBreakdown),
    [safeBreakdown],
  );

  const hasData =
    stats.total > 0 || availabilityPct !== undefined || healthPct !== undefined;

  const handleOpen = (target: HTMLElement) => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    if (!disabled && hasData) {
      setAnchorEl(target);
    }
  };

  const handleClose = () => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
    }
    closeTimerRef.current = setTimeout(() => {
      setAnchorEl(null);
    }, 120);
  };

  const handleCloseImmediately = () => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    setAnchorEl(null);
  };

  useEffect(() => {
    return () => {
      if (closeTimerRef.current) {
        clearTimeout(closeTimerRef.current);
      }
    };
  }, []);

  const open = Boolean(anchorEl && hasData && !disabled);

  const childProps = children.props as {
    onMouseEnter?: (e: React.MouseEvent<HTMLElement>) => void;
    onMouseLeave?: (e: React.MouseEvent<HTMLElement>) => void;
    onClick?: (e: React.MouseEvent<HTMLElement>) => void;
  };

  const trigger = cloneElement<React.HTMLAttributes<HTMLElement>>(children, {
    onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
      childProps.onMouseEnter?.(e);
      handleOpen(e.currentTarget);
    },
    onMouseLeave: (e: React.MouseEvent<HTMLElement>) => {
      childProps.onMouseLeave?.(e);
      handleClose();
    },
    onClick: (e: React.MouseEvent<HTMLElement>) => {
      childProps.onClick?.(e);
      if (open) {
        handleCloseImmediately();
      } else {
        handleOpen(e.currentTarget);
      }
    },
  });

  return (
    <>
      {trigger}
      {hasData && !disabled && (
        <Popover
          open={open}
          anchorEl={anchorEl}
          onClose={handleCloseImmediately}
          anchorOrigin={anchorOrigin}
          transformOrigin={transformOrigin}
          transitionDuration={transitionDuration}
          disableRestoreFocus
          slotProps={{
            paper: {
              onMouseEnter: () => {
                if (closeTimerRef.current) {
                  clearTimeout(closeTimerRef.current);
                  closeTimerRef.current = null;
                }
              },
              onMouseLeave: handleClose,
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
            breakdown={safeBreakdown}
            availabilityPct={availabilityPct}
            healthPct={healthPct}
            baseFilter={baseFilter}
          />
        </Popover>
      )}
    </>
  );
};
