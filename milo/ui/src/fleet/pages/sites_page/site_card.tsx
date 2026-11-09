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
import * as tk from '@/fleet/theme/tokens';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { formatSiteUpdatedAt, getCampusMapsUrl } from './site_helpers';
import { SitePlatformRollup } from './site_platform_rollup';

export interface SiteCardProps {
  readonly site: PhysicalSite;
  readonly onEdit?: (site: PhysicalSite) => void;
  readonly onDelete?: (site: PhysicalSite) => void;
}

const ctrlIconSx = { fontSize: tk.iconSizes.control } as const;
const rowBetweenSx = {
  ...actionRowSx,
  justifyContent: 'space-between',
  flexWrap: 'nowrap',
  gap: 1,
} as const;
const cardSx = {
  ...surfaceCardSx,
  p: 2.25,
  display: 'flex',
  flexDirection: 'column',
  gap: 1.25,
} as const;
const titleRowSx = {
  ...actionRowSx,
  gap: 1,
  flex: 1,
  minWidth: 0,
  overflow: 'hidden',
  flexWrap: 'nowrap',
} as const;
const placeIconSx = {
  fontSize: tk.iconSizes.inline,
  color: tk.fleetColors.primary,
  flexShrink: 0,
} as const;
const pillInnerSx = {
  ...actionRowSx,
  gap: 0.5,
  fontFamily: tk.fontFamilies.mono,
  color: tk.fleetColors.primary,
} as const;

export const SiteCard = ({ site, onEdit, onDelete }: SiteCardProps) => {
  const campusMapsUrl = getCampusMapsUrl(site.id);
  const formattedUpdatedAt = formatSiteUpdatedAt(site.updatedAt);

  return (
    <Box
      component="article"
      aria-label={site.name}
      data-testid={`site-card-${site.id}`}
      sx={cardSx}
    >
      <Box sx={{ ...rowBetweenSx, minHeight: 32 }}>
        <Box sx={titleRowSx}>
          <PlaceOutlinedIcon sx={placeIconSx} />
          <Tooltip title={site.name} enterDelay={500}>
            <Typography
              variant="subtitle1"
              component="h2"
              noWrap
              sx={{ fontWeight: 600, minWidth: 0, flex: 1 }}
            >
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
                  <EditOutlinedIcon sx={ctrlIconSx} />
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
                  <DeleteOutlineIcon sx={ctrlIconSx} />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        )}
      </Box>

      <SitePlatformRollup
        platformFilters={site.platformFilters}
        onAddFilters={onEdit ? () => onEdit(site) : undefined}
      />

      <Box
        sx={{
          ...rowBetweenSx,
          mt: 'auto',
          pt: 1.25,
          borderTop: tk.borders.divider,
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
              sx={{ display: 'inline-flex', borderRadius: tk.radii.pill }}
            >
              <StatusPill tone="neutral" dot={false}>
                <Box component="span" sx={pillInnerSx}>
                  {site.id}
                  <OpenInNewIcon sx={{ fontSize: 12 }} />
                </Box>
              </StatusPill>
            </Link>
          </Tooltip>
        ) : (
          <StatusPill tone="neutral" dot={false}>
            <Box component="span" sx={{ fontFamily: tk.fontFamilies.mono }}>
              {site.id}
            </Box>
          </StatusPill>
        )}
        {formattedUpdatedAt && (
          <Typography variant="caption" color="text.secondary" noWrap>
            {`Updated ${formattedUpdatedAt}`}
          </Typography>
        )}
      </Box>
    </Box>
  );
};
