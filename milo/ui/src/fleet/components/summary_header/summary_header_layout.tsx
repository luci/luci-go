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

import { Box, SxProps, Theme, Typography } from '@mui/material';
import { ReactNode } from 'react';

export interface SummaryHeaderTitleProps {
  children: ReactNode;
  sx?: SxProps<Theme>;
}

/**
 * SummaryHeaderTitle renders the shared h5 title for device summary headers.
 */
export function SummaryHeaderTitle({ children, sx }: SummaryHeaderTitleProps) {
  return (
    <Typography
      variant="h5"
      sx={{
        mb: 1,
        color: (theme) => theme.palette.text.primary,
        ...sx,
      }}
    >
      {children}
    </Typography>
  );
}

export interface SubMetricListProps {
  children: ReactNode;
  sx?: SxProps<Theme>;
}

/**
 * SubMetricList renders a column list container for small metric items beneath a metric card.
 */
export function SubMetricList({ children, sx }: SubMetricListProps) {
  return (
    <Box
      sx={{
        mt: 0.5,
        px: 1,
        display: 'flex',
        flexDirection: 'column',
        ...sx,
      }}
    >
      {children}
    </Box>
  );
}
