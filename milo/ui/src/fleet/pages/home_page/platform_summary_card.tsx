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

import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Link,
  Skeleton,
  Typography,
} from '@mui/material';
import { type ReactNode } from 'react';
import { NavLink } from 'react-router';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { colors } from '@/fleet/theme/colors';

// Health thresholds aligned with Fleet Console UX standards to indicate
// platform health status (Healthy >= 90%, Warning >= 75%, Danger < 75%).
const HEALTH_THRESHOLD_HIGH = 90;
const HEALTH_THRESHOLD_WARN = 75;

const HEALTH_COLOR_MAP = {
  high: { bg: colors.green[100], text: colors.green[800] },
  warn: { bg: colors.yellow[100], text: colors.yellow[900] },
  low: { bg: colors.red[100], text: colors.red[800] },
} as const;

const PermissionWarningTooltip = () => (
  <InfoTooltip color="warning.main" fontSize="1.25rem">
    <Typography variant="body2">
      <strong>
        You may not have the proper permissions to see the devices.
      </strong>
      <br />
      <br />
      Go to{' '}
      <Link
        href="http://go/fcon-user-guide#getting-access"
        target="_blank"
        rel="noreferrer"
        sx={{ textDecoration: 'underline' }}
      >
        go/fcon-user-guide#getting-access
      </Link>{' '}
      to read more on how to get those permissions.
    </Typography>
  </InfoTooltip>
);

export interface PlatformSummaryCardProps {
  title: string;
  logoSrc?: string;
  total?: number;
  totalText?: string;
  isLoading?: boolean;
  isError?: boolean;
  linkTo: string;
  linkText: string;
  linkIcon?: ReactNode;
  secondaryLinkTo?: string;
  secondaryLinkText?: string;
  secondaryLinkIcon?: ReactNode;
  secondTotal?: number;
  secondTotalText?: string;
  healthyPercentage?: number;
}

export function PlatformSummaryCard({
  title,
  logoSrc,
  total,
  totalText,
  isLoading,
  isError,
  linkTo,
  linkText,
  linkIcon,
  secondaryLinkTo,
  secondaryLinkText,
  secondaryLinkIcon,
  secondTotal,
  secondTotalText,
  healthyPercentage,
}: PlatformSummaryCardProps) {
  const roundedPercentage =
    healthyPercentage !== undefined
      ? Math.round(healthyPercentage * 10) / 10
      : undefined;

  const status =
    roundedPercentage !== undefined
      ? roundedPercentage >= HEALTH_THRESHOLD_HIGH
        ? 'high'
        : roundedPercentage >= HEALTH_THRESHOLD_WARN
          ? 'warn'
          : 'low'
      : null;

  return (
    <Card
      variant="outlined"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 200,
        boxShadow: 'none',
        borderColor: colors.grey[300],
      }}
    >
      <CardContent>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 2,
            mb: 2,
          }}
        >
          {logoSrc && (
            <img
              src={logoSrc}
              alt={`${title} logo`}
              style={{ width: 48, height: 48, objectFit: 'contain' }}
            />
          )}
          <Typography variant="h5" component="h2" sx={{ m: 0 }}>
            {title}
          </Typography>
          {roundedPercentage !== undefined && (
            <Box
              sx={{ display: 'flex', alignItems: 'center', gap: 1, ml: 'auto' }}
            >
              <Chip
                size="small"
                label={`${roundedPercentage.toFixed(1)}% Healthy`}
                sx={{
                  bgcolor: status ? HEALTH_COLOR_MAP[status].bg : undefined,
                  color: status ? HEALTH_COLOR_MAP[status].text : undefined,
                  fontWeight: 500,
                }}
              />
            </Box>
          )}
        </Box>

        {isError ? (
          <Typography color="error" variant="body2" sx={{ mt: 2 }}>
            Error loading data
          </Typography>
        ) : (
          <>
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'space-between',
                gap: 3,
                mt: 4,
              }}
            >
              {linkIcon && (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    color: 'text.secondary',
                    '& svg': { fontSize: '2rem' },
                  }}
                >
                  {linkIcon}
                </Box>
              )}
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Button
                  variant="text"
                  component={NavLink}
                  to={linkTo}
                  sx={{
                    color: 'inherit',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                  }}
                >
                  <Typography
                    variant="h3"
                    component="div"
                    sx={{ mb: 0.5, minWidth: 60 }}
                  >
                    {isLoading ? (
                      <Skeleton variant="text" width="100%" />
                    ) : (
                      (total?.toLocaleString('en-US') ?? 0)
                    )}
                  </Typography>
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 1,
                    }}
                  >
                    <Typography color="text.secondary" variant="body2">
                      {totalText ?? 'Total Devices'}
                    </Typography>
                  </Box>
                </Button>
                {!isLoading && total === 0 && <PermissionWarningTooltip />}
              </Box>
              <Box
                sx={{
                  display: 'flex',
                  gap: 1,
                  alignItems: 'center',
                }}
              >
                <Button
                  component={NavLink}
                  to={linkTo}
                  variant="outlined"
                  disableElevation
                  fullWidth
                >
                  {linkText}
                </Button>
              </Box>
            </Box>
            {secondaryLinkTo && (
              <Box
                sx={{
                  mt: 3,
                  display: 'flex',
                  justifyContent: 'space-between',
                  gap: 3,
                }}
              >
                {secondaryLinkIcon && (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      color: 'text.secondary',
                      '& svg': { fontSize: '2rem' },
                    }}
                  >
                    {secondaryLinkIcon}
                  </Box>
                )}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Button
                    variant="text"
                    component={NavLink}
                    to={secondaryLinkTo}
                    sx={{
                      textTransform: 'none',
                      color: 'inherit',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'flex-start',
                      padding: 1,
                      minWidth: 0,
                      borderRadius: 1,
                    }}
                  >
                    <Typography
                      variant="h3"
                      component="div"
                      sx={{ mb: 0.5, minWidth: 60 }}
                    >
                      {isLoading ? (
                        <Skeleton variant="text" width="100%" />
                      ) : (
                        (secondTotal?.toLocaleString('en-US') ?? 0)
                      )}
                    </Typography>
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                      }}
                    >
                      <Typography color="text.secondary" variant="body2">
                        {secondTotalText ?? 'Total Devices'}
                      </Typography>
                    </Box>
                  </Button>
                  {!isLoading && secondTotal === 0 && (
                    <PermissionWarningTooltip />
                  )}
                </Box>
                <Box
                  sx={{
                    display: 'flex',
                    gap: 1,
                    alignItems: 'center',
                  }}
                >
                  <Button
                    component={NavLink}
                    to={secondaryLinkTo}
                    variant="outlined"
                    disableElevation
                    fullWidth
                  >
                    {secondaryLinkText}
                  </Button>
                </Box>
              </Box>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
