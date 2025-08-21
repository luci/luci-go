// Copyright 2025 The LUCI Authors.
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

import { CSSObject } from '@emotion/react';
import { InfoOutlined } from '@mui/icons-material';
import { Popover, Typography } from '@mui/material';
import React, { useRef, useState } from 'react';

/**
 * Renders an interactive hovercard.
 * Uses a timer-based delay to allow the user's mouse to travel from the
 * trigger icon to the popover content without it closing prematurely.
 */
export function InfoTooltip({
  children,
  infoCss = {},
}: React.PropsWithChildren<{ infoCss?: CSSObject }>) {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const open = Boolean(anchorEl);
  const popoverId = open ? 'info-tooltip-state-popover' : undefined;
  const timerRef = useRef<number | null>(null);

  const handlePopoverOpen = (event: React.MouseEvent<HTMLElement>) => {
    if (timerRef.current !== null) {
      // Clear any pending 'close' timer.
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (anchorEl === null) {
      setAnchorEl(event.currentTarget);
    }
  };

  const handlePopoverClose = () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
    }
    // Schedules the popover to close after a short delay.
    // This gives the user time to move their mouse from the info icon into the popover.
    timerRef.current = window.setTimeout(() => setAnchorEl(null), 200);
  };

  return (
    <>
      <span
        aria-owns={popoverId}
        aria-haspopup="true"
        onMouseEnter={handlePopoverOpen}
        onMouseLeave={handlePopoverClose}
        css={[
          {
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
          },
          infoCss,
        ]}
      >
        <InfoOutlined sx={{ fontSize: '1rem', color: 'action.active' }} />
      </span>
      <Popover
        id={popoverId}
        sx={{
          pointerEvents: 'none',
        }}
        open={open}
        anchorEl={anchorEl}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
        // Prevents the popover from grabbing focus, which is important for
        // hover-activated elements and fixes accessibility warnings.
        disableAutoFocus
        slotProps={{
          paper: {
            onMouseEnter: handlePopoverOpen,
            onMouseLeave: handlePopoverClose,
            sx: {
              pointerEvents: 'auto',
              p: 2,
              border: (theme) => `1px solid ${theme.palette.divider}`,
              boxShadow: 3,
              maxWidth: 450,
            },
          },
        }}
      >
        {typeof children === 'string' ? (
          <Typography variant="body2">{children}</Typography>
        ) : (
          children
        )}
      </Popover>
    </>
  );
}
