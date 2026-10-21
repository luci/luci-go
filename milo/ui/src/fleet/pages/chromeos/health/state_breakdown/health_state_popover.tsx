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

import { Box, Fade, Popper, PopperPlacementType } from '@mui/material';
import { cloneElement, useMemo } from 'react';

import {
  computeBreakdownStats,
  DeviceStateBreakdown,
} from './compute_breakdown_stats';
import { HealthStatePopoverContent } from './health_state_popover_content';
import { usePopoverHover, VirtualElement } from './use_popover_hover';

export type { VirtualElement };

export interface HealthStatePopoverProps {
  readonly title: string;
  readonly breakdown?: DeviceStateBreakdown | undefined;
  readonly availabilityPct?: number | undefined;
  readonly healthPct?: number | undefined;
  readonly baseFilter?: string | undefined;
  readonly disabled?: boolean;
  readonly enterDelay?: number;
  readonly leaveDelay?: number;
  readonly placement?: PopperPlacementType;
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

/**
 * Reusable hovercard popover wrapper that displays the complete DUT state breakdown
 * and horizontal SLA threshold status for any trigger element (e.g. status labels or progress bars).
 * Uses Popper and usePopoverHover for clean cursor anchoring and scroll dismissal.
 */
export const HealthStatePopover = ({
  title,
  breakdown,
  availabilityPct,
  healthPct,
  baseFilter,
  disabled = false,
  enterDelay = 120,
  leaveDelay = 200,
  placement = 'bottom',
  transitionDuration = 150,
  children,
}: HealthStatePopoverProps) => {
  const safeBreakdown = breakdown ?? EMPTY_BREAKDOWN;
  const stats = useMemo(
    () => computeBreakdownStats(safeBreakdown),
    [safeBreakdown],
  );

  const hasData =
    stats.total > 0 || availabilityPct !== undefined || healthPct !== undefined;
  const enabled = !disabled && hasData;

  const {
    anchorEl,
    open,
    handleOpen,
    handleClose,
    cancelClose,
    handleCloseImmediately,
  } = usePopoverHover({
    enabled,
    enterDelay,
    leaveDelay,
  });

  const childProps = children.props as {
    onMouseEnter?: (e: React.MouseEvent<HTMLElement>) => void;
    onMouseLeave?: (e: React.MouseEvent<HTMLElement>) => void;
    onClick?: (e: React.MouseEvent<HTMLElement>) => void;
  };

  const trigger = cloneElement<React.HTMLAttributes<HTMLElement>>(children, {
    onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
      childProps.onMouseEnter?.(e);
      handleOpen(e.currentTarget, e.clientX);
    },
    onMouseLeave: (e: React.MouseEvent<HTMLElement>) => {
      childProps.onMouseLeave?.(e);
      handleClose();
    },
    onClick: (e: React.MouseEvent<HTMLElement>) => {
      e.stopPropagation();
      childProps.onClick?.(e);
      if (open) {
        handleCloseImmediately();
      } else {
        handleOpen(e.currentTarget, e.clientX, 0);
      }
    },
  });

  const timeout =
    typeof transitionDuration === 'number'
      ? transitionDuration
      : (transitionDuration?.enter ?? 150);

  return (
    <>
      {trigger}
      {enabled && (
        <Popper
          open={open}
          anchorEl={anchorEl}
          placement={placement}
          transition
          modifiers={[
            {
              name: 'offset',
              options: {
                offset: [0, 8],
              },
            },
            {
              name: 'flip',
              options: {
                fallbackPlacements: ['top', 'bottom'],
              },
            },
            {
              name: 'preventOverflow',
              options: {
                boundary: 'window',
                padding: 16,
              },
            },
          ]}
          sx={{
            zIndex: (theme) => theme.zIndex.tooltip,
            pointerEvents: 'none',
          }}
        >
          {({ TransitionProps }) => (
            <Fade {...TransitionProps} timeout={timeout}>
              <Box
                sx={{ pointerEvents: 'auto' }}
                onMouseEnter={cancelClose}
                onMouseLeave={handleClose}
              >
                <HealthStatePopoverContent
                  title={title}
                  breakdown={safeBreakdown}
                  availabilityPct={availabilityPct}
                  healthPct={healthPct}
                  baseFilter={baseFilter}
                />
              </Box>
            </Fade>
          )}
        </Popper>
      )}
    </>
  );
};
