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
  healthChipSuffix?: string;
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
  healthChipSuffix,
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

  const hasAnyIcon = Boolean(linkIcon || secondaryLinkIcon);

  return (
    <Card
      variant="outlined"
      sx={{
        containerType: 'inline-size',
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
                label={`${roundedPercentage.toFixed(1)}% ${healthChipSuffix ?? 'Healthy'}`}
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
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: hasAnyIcon
                ? 'auto minmax(8px, 1fr) auto minmax(8px, 1fr) auto'
                : '1fr auto',
              alignItems: 'center',
              columnGap: hasAnyIcon ? 0 : 3,
              rowGap: 3,
              mt: 4,
              '@container (max-width: 320px)': {
                gridTemplateColumns: hasAnyIcon ? 'auto 1fr' : '1fr',
                rowGap: 1.5,
              },
            }}
          >
            {hasAnyIcon &&
              (linkIcon ? (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    color: 'text.secondary',
                    '& svg': { fontSize: '2rem' },
                    gridColumn: 1,
                  }}
                >
                  {linkIcon}
                </Box>
              ) : (
                <Box sx={{ gridColumn: 1 }} />
              ))}
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                minWidth: 'max-content',
                gridColumn: hasAnyIcon ? 3 : 1,
                '@container (max-width: 320px)': {
                  gridColumn: hasAnyIcon ? 2 : 1,
                  ml: hasAnyIcon ? 1 : 0,
                },
              }}
            >
              <Button
                variant="text"
                component={NavLink}
                to={linkTo}
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
                    (total?.toLocaleString('en-US') ?? 0)
                  )}
                </Typography>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                  }}
                  aria-label={
                    totalText
                      ? `${totalText} in ${title} platform`
                      : `Total Devices in ${title} platform`
                  }
                >
                  <Typography color="text.secondary" variant="body2" noWrap>
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
                justifyContent: 'flex-end',
                width: '100%',
                minWidth: 'max-content',
                gridColumn: hasAnyIcon ? 5 : 2,
                '@container (max-width: 320px)': {
                  gridColumn: '1 / -1',
                  minWidth: 0,
                  mb: 1,
                },
              }}
            >
              <Button
                component={NavLink}
                to={linkTo}
                variant="outlined"
                disableElevation
                fullWidth
                aria-label={`${linkText} in ${title} platform`}
                sx={{
                  whiteSpace: 'nowrap',
                }}
              >
                {linkText}
              </Button>
            </Box>
            {secondaryLinkTo && (
              <>
                {hasAnyIcon &&
                  (secondaryLinkIcon ? (
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        color: 'text.secondary',
                        '& svg': { fontSize: '2rem' },
                        gridColumn: 1,
                      }}
                    >
                      {secondaryLinkIcon}
                    </Box>
                  ) : (
                    <Box sx={{ gridColumn: 1 }} />
                  ))}
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    minWidth: 'max-content',
                    gridColumn: hasAnyIcon ? 3 : 1,
                    '@container (max-width: 320px)': {
                      gridColumn: hasAnyIcon ? 2 : 1,
                      ml: hasAnyIcon ? 1 : 0,
                    },
                  }}
                >
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
                      aria-label={
                        secondTotalText
                          ? `${secondTotalText} in ${title} platform`
                          : `Total Devices in ${title} platform`
                      }
                    >
                      <Typography color="text.secondary" variant="body2" noWrap>
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
                    justifyContent: 'flex-end',
                    width: '100%',
                    minWidth: 'max-content',
                    gridColumn: hasAnyIcon ? 5 : 2,
                    '@container (max-width: 320px)': {
                      gridColumn: '1 / -1',
                      minWidth: 0,
                      mb: 1,
                    },
                  }}
                >
                  <Button
                    component={NavLink}
                    to={secondaryLinkTo}
                    variant="outlined"
                    disableElevation
                    fullWidth
                    aria-label={`${secondaryLinkText} in ${title} platform`}
                    sx={{
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {secondaryLinkText}
                  </Button>
                </Box>
              </>
            )}
          </Box>
        )}
      </CardContent>
    </Card>
  );
}
