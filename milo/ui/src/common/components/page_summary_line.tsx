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

import { Box, Typography } from '@mui/material';
import { Children, Fragment } from 'react';

interface PageSummaryLineProps {
  children: React.ReactNode;
}

/**
 * Renders a line of summary items for a page, intended to be a line below the PageTitle component.
 * Each item is separated by dividers.
 */
export function PageSummaryLine({ children }: PageSummaryLineProps) {
  const validChildren = Children.toArray(children).filter(Boolean);

  return (
    <Typography
      component="div"
      variant="body2"
      sx={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        rowGap: '4px',
      }}
    >
      {validChildren.map((child, index) => (
        <Fragment key={index}>
          {child}
          {index < validChildren.length - 1 && <SummaryLineDivider />}
        </Fragment>
      ))}
    </Typography>
  );
}

interface SummaryLineItemProps {
  label: string;
  icon?: React.ReactNode;
  children: React.ReactNode;
}

/**
 * A single item within a PageSummaryLine.
 */
export function SummaryLineItem({ label, children }: SummaryLineItemProps) {
  return (
    <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center' }}>
      <Typography variant="body2" component="span" color="text.secondary">
        {label}:
      </Typography>
      <Typography
        variant="body2"
        component="span"
        color="text.primary"
        sx={{ ml: 0.5 }}
      >
        {children}
      </Typography>
    </Box>
  );
}

function SummaryLineDivider() {
  return (
    <Box
      component="span"
      sx={{
        mx: 0.75,
        color: 'text.disabled',
        alignSelf: 'center',
        lineHeight: 'initial',
      }}
    >
      |
    </Box>
  );
}
