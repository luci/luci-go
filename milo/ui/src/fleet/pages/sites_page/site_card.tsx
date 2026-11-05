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

import PlaceOutlinedIcon from '@mui/icons-material/PlaceOutlined';
import { Box, Tooltip, Typography } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import { actionRowSx, surfaceCardSx } from '@/fleet/theme/recipes';
import { fleetColors, fontFamilies, iconSizes } from '@/fleet/theme/tokens';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface SiteCardProps {
  readonly site: PhysicalSite;
}

export const SiteCard = ({ site }: SiteCardProps) => {
  return (
    <Box
      component="article"
      aria-label={site.name}
      data-testid={`site-card-${site.id}`}
      sx={{
        ...surfaceCardSx,
        p: 2.25,
        display: 'flex',
        flexDirection: 'column',
        gap: 1.25,
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: 1,
        }}
      >
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            minWidth: 0,
          }}
        >
          <PlaceOutlinedIcon
            sx={{
              fontSize: iconSizes.inline,
              color: fleetColors.primary,
              flexShrink: 0,
            }}
          />
          <Tooltip title={site.name} enterDelay={500}>
            <Typography variant="h6" component="h2" noWrap>
              {site.name}
            </Typography>
          </Tooltip>
        </Box>
      </Box>

      <Box sx={actionRowSx}>
        <StatusPill tone="neutral" dot={false}>
          <Box component="span" sx={{ fontFamily: fontFamilies.mono }}>
            {site.id}
          </Box>
        </StatusPill>
      </Box>
    </Box>
  );
};
