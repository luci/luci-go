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
  CardActions,
  CardContent,
  Skeleton,
  Typography,
} from '@mui/material';
import { NavLink } from 'react-router';

import { colors } from '@/fleet/theme/colors';

export interface PlatformSummaryCardProps {
  title: string;
  logoSrc?: string;
  total?: number;
  isLoading?: boolean;
  isError?: boolean;
  linkTo: string;
  linkText: string;
  secondaryLinkTo?: string;
  secondaryLinkText?: string;
}

export function PlatformSummaryCard({
  title,
  logoSrc,
  total,
  isLoading,
  isError,
  linkTo,
  linkText,
  secondaryLinkTo,
  secondaryLinkText,
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
      <CardContent sx={{ flexGrow: 1 }}>
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
          <Box sx={{ mt: 2 }}>
            <Typography variant="h3" component="div" sx={{ mb: 1 }}>
              {total?.toLocaleString() ?? 0}
            </Typography>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
              }}
            >
              <Typography color="text.secondary">Total Devices</Typography>
            </Box>
          </Box>
        )}
      </CardContent>
      <CardActions sx={{ p: 2, pt: 0, gap: 1 }}>
        <Button
          component={NavLink}
          to={linkTo}
          variant="contained"
          disableElevation
          fullWidth
        >
          {linkText}
        </Button>
        {secondaryLinkTo && secondaryLinkText && (
          <Button
            component={NavLink}
            to={secondaryLinkTo}
            variant="outlined"
            disableElevation
            fullWidth
          >
            {secondaryLinkText}
          </Button>
        )}
      </CardActions>
    </Card>
  );
}
