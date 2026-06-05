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
import { Tooltip, Typography } from '@mui/material';
import React from 'react';

/**
 * Renders an interactive hovercard.
 * Uses MUI Tooltip with interactive prop to handle hover delays
 * and enter/leave behavior out of the box.
 */
export function InfoTooltip({
  children,
  infoCss = {},
  paperCss = {},
  color = 'action.active',
  fontSize = '1rem',
}: React.PropsWithChildren<{
  infoCss?: CSSObject;
  paperCss?: CSSObject;
  color?: string;
  fontSize?: string;
}>) {
  return (
    <Tooltip
      title={
        <div
          onClick={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
          onKeyDown={(e) => e.stopPropagation()}
          role="presentation"
        >
          {typeof children === 'string' ? (
            <Typography variant="body2">{children}</Typography>
          ) : (
            children
          )}
        </div>
      }
      leaveDelay={200}
      placement="bottom-start"
      slotProps={{
        tooltip: {
          sx: [
            {
              bgcolor: 'background.paper',
              color: 'text.primary',
              p: 2,
              border: (theme) => `1px solid ${theme.palette.divider}`,
              boxShadow: 3,
              maxWidth: 450,
              pointerEvents: 'auto',
              fontSize: '0.875rem',
            },
            paperCss,
          ],
        },
      }}
    >
      <span
        className="fleet-info-icon"
        css={[
          {
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
          },
          infoCss,
        ]}
      >
        <InfoOutlined sx={{ fontSize, color }} />
      </span>
    </Tooltip>
  );
}
