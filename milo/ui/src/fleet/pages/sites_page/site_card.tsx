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

import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import PlaceOutlinedIcon from '@mui/icons-material/PlaceOutlined';
import { Box, IconButton, Link, Tooltip, Typography } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  actionRowSx,
  iconButtonSx,
  surfaceCardSx,
} from '@/fleet/theme/recipes';
import {
  fleetColors,
  fontFamilies,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getCampusMapsUrl } from './site_helpers';

export interface SiteCardProps {
  readonly site: PhysicalSite;
  readonly onEdit?: (site: PhysicalSite) => void;
  readonly onDelete?: (site: PhysicalSite) => void;
}

export const SiteCard = ({ site, onEdit, onDelete }: SiteCardProps) => {
  const campusMapsUrl = getCampusMapsUrl(site.id);

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

        {(onEdit || onDelete) && (
          <Box sx={{ ...actionRowSx, gap: 0.5, flexShrink: 0 }}>
            {onEdit && (
              <Tooltip title="Edit site">
                <IconButton
                  size="small"
                  aria-label={`Edit ${site.name}`}
                  onClick={() => onEdit(site)}
                  sx={iconButtonSx}
                >
                  <EditOutlinedIcon sx={{ fontSize: iconSizes.control }} />
                </IconButton>
              </Tooltip>
            )}
            {onDelete && (
              <Tooltip title="Delete site">
                <IconButton
                  size="small"
                  aria-label={`Delete ${site.name}`}
                  onClick={() => onDelete(site)}
                  sx={iconButtonSx}
                >
                  <DeleteOutlineIcon sx={{ fontSize: iconSizes.control }} />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        )}
      </Box>

      <Box
        sx={{
          ...actionRowSx,
          mt: 'auto',
          justifyContent: 'space-between',
          flexWrap: 'nowrap',
          gap: 1,
        }}
      >
        {campusMapsUrl ? (
          <Tooltip title="Open in Campus Maps">
            <Link
              href={campusMapsUrl}
              target="_blank"
              rel="noopener noreferrer"
              underline="none"
              aria-label={`Open ${site.id} in Campus Maps`}
              sx={{ display: 'inline-flex', borderRadius: radii.pill }}
            >
              <StatusPill tone="neutral" dot={false}>
                <Box
                  component="span"
                  sx={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 0.5,
                    fontFamily: fontFamilies.mono,
                    color: fleetColors.primary,
                  }}
                >
                  {site.id}
                  <OpenInNewIcon sx={{ fontSize: 12 }} />
                </Box>
              </StatusPill>
            </Link>
          </Tooltip>
        ) : (
          <StatusPill tone="neutral" dot={false}>
            <Box component="span" sx={{ fontFamily: fontFamilies.mono }}>
              {site.id}
            </Box>
          </StatusPill>
        )}
      </Box>
    </Box>
  );
};
