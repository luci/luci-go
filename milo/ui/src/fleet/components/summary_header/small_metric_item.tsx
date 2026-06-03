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

import { Box, ButtonBase, Skeleton, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import React from 'react';

export type SmallMetricItemProps = {
  value?: number;
  total?: number;
  onClick?: () => void;
  dotColor?: string;
  loading?: boolean;
  formatValue?: (val: number) => string;
} & (
  | {
      label: string;
      /**
       * Accessible label for the item. If omitted, it will be derived from the label by removing colons.
       */
      ariaLabel?: string;
    }
  | {
      label: React.ReactNode;
      /**
       * Accessible label for the item. Required if label is a non-string node.
       */
      ariaLabel: string;
    }
);

export function SmallMetricItem({
  label,
  value,
  total,
  onClick,
  dotColor,
  loading,
  formatValue,
  ariaLabel,
}: SmallMetricItemProps) {
  const theme = useTheme();
  const percentage =
    !loading && typeof value === 'number' && total !== undefined && total > 0
      ? ((value / total) * 100).toFixed(1)
      : undefined;

  return (
    <ButtonBase
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      aria-label={
        ariaLabel ||
        (typeof label === 'string' ? label.replace(':', '') : undefined)
      }
      sx={{
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'wrap',
        gap: 0.5,
        cursor: onClick ? 'pointer' : 'default',
        width: '100%',
        justifyContent: 'flex-start',
        textAlign: 'left',
        ...(onClick && {
          '&:hover, &:focus-visible': {
            backgroundColor: theme.palette.action.hover,
            borderRadius: '4px',
            outline: 'none',
          },
          '&:hover .small-metric-label, &:focus-visible .small-metric-label': {
            textDecoration: 'underline',
          },
        }),
        p: '1px 4px',
        minHeight: '1rem',
      }}
    >
      {dotColor && (
        <Box
          sx={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: dotColor,
            flexShrink: 0,
            mr: 0.5,
          }}
        />
      )}
      <Typography
        component="div"
        variant="body2"
        className="small-metric-label"
        sx={{
          color: 'text.secondary',
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
        }}
      >
        {label}
      </Typography>
      {loading ? (
        <Skeleton width={30} height={16} sx={{ ml: 0.5 }} />
      ) : (
        <Box
          component="span"
          sx={{
            display: 'inline-flex',
            alignItems: 'baseline',
            gap: 0.5,
            ml: 0.5,
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <Typography
            variant="body2"
            sx={{
              color: 'text.primary',
              fontWeight: 700,
            }}
          >
            {formatValue
              ? formatValue(value ?? 0)
              : (value ?? 0).toLocaleString()}
          </Typography>
          {percentage !== undefined && (
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: 300,
                fontSize: '0.75rem',
                opacity: 0.7,
              }}
            >
              ({percentage}%)
            </Typography>
          )}
        </Box>
      )}
    </ButtonBase>
  );
}
