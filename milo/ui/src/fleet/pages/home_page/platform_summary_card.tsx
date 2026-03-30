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
  Skeleton,
  Typography,
} from '@mui/material';
import { type ReactNode } from 'react';
import { NavLink } from 'react-router';

import { colors } from '@/fleet/theme/colors';

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
}: PlatformSummaryCardProps) {
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
        </Box>

        {isLoading ? (
          <Box sx={{ mt: 2 }}>
            <Typography variant="h3" component="div" sx={{ mb: 1 }}>
              <Skeleton width="40%" />
            </Typography>
            <Typography color="text.secondary" gutterBottom>
              <Skeleton width="30%" />
            </Typography>
          </Box>
        ) : isError ? (
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
                <Typography variant="h3" component="div" sx={{ mb: 0.5 }}>
                  {total?.toLocaleString('en-US') ?? 0}
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
                  <Typography variant="h3" component="div" sx={{ mb: 0.5 }}>
                    {secondTotal?.toLocaleString('en-US') ?? 0}
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
