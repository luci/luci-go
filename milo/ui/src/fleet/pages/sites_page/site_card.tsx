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

import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import PhotoCameraOutlinedIcon from '@mui/icons-material/PhotoCameraOutlined';
import PlaceOutlinedIcon from '@mui/icons-material/PlaceOutlined';
import { Box, IconButton, Link, Tooltip, Typography } from '@mui/material';
import { useState } from 'react';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  actionRowSx,
  iconButtonSx,
  surfaceCardClippedSx,
} from '@/fleet/theme/recipes';
import * as tk from '@/fleet/theme/tokens';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  formatSiteUpdatedAt,
  getCampusMapsUrl,
  normalizeSiteImageUrl,
} from './site_helpers';
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
  ...surfaceCardClippedSx,
  display: 'flex',
  flexDirection: 'column',
} as const;
const cardBodySx = {
  p: 2.25,
  display: 'flex',
  flexDirection: 'column',
  gap: 1.5,
  flex: 1,
} as const;
const footerSx = {
  ...rowBetweenSx,
  mt: 'auto',
  pt: 1.5,
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
const navBtnSx = {
  ...iconButtonSx,
  backgroundColor: tk.fleetColors.surface,
  border: tk.borders.divider,
  pointerEvents: 'auto',
  '&:hover': { backgroundColor: tk.fleetColors.surfaceTonal },
} as const;
const photoBadgeSx = {
  ...actionRowSx,
  gap: 0.5,
  px: 1,
  py: 0.25,
  borderRadius: tk.radii.pill,
  backgroundColor: tk.fleetColors.surface,
  border: tk.borders.divider,
  color: tk.fleetColors.textPrimary,
} as const;
const carouselBoxSx = {
  position: 'relative',
  width: '100%',
  height: 172,
  overflow: 'hidden',
  borderBottom: tk.borders.divider,
  backgroundColor: tk.fleetColors.surfaceTonal,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  flexShrink: 0,
} as const;
const placeholderStackSx = {
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 0.5,
  color: tk.fleetColors.textTertiary,
} as const;

export const SiteCard = ({ site, onEdit, onDelete }: SiteCardProps) => {
  const [photoIdx, setPhotoIdx] = useState(0);
  const [failedUrls, setFailedUrls] = useState<Record<string, boolean>>({});
  const validUrls = Array.from(
    new Set(
      (site.imageUrls ?? [])
        .map(normalizeSiteImageUrl)
        .filter((u): u is string => u !== null && !failedUrls[u]),
    ),
  );
  const safeIdx = validUrls.length > 0 ? photoIdx % validUrls.length : 0;
  const activeUrl = validUrls[safeIdx] ?? null;

  const campusMapsUrl = getCampusMapsUrl(site.id);
  const formattedUpdatedAt = formatSiteUpdatedAt(site.updatedAt);

  return (
    <Box
      component="article"
      aria-label={site.name}
      data-testid={`site-card-${site.id}`}
      sx={cardSx}
    >
      {activeUrl ? (
        <Box data-testid={`site-photo-carousel-${site.id}`} sx={carouselBoxSx}>
          <Box
            component="img"
            src={activeUrl}
            alt={`${site.name} facility photo ${safeIdx + 1}`}
            loading="lazy"
            onError={() =>
              setFailedUrls((prev) => ({ ...prev, [activeUrl]: true }))
            }
            sx={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
          {validUrls.length > 1 && (
            <Box
              sx={{
                position: 'absolute',
                bottom: 10,
                left: 10,
                right: 10,
                ...rowBetweenSx,
                pointerEvents: 'none',
              }}
            >
              <Box sx={{ ...actionRowSx, gap: 0.75, pointerEvents: 'auto' }}>
                <IconButton
                  size="small"
                  aria-label={`Previous photo for ${site.name}`}
                  onClick={() =>
                    setPhotoIdx(
                      (i) => (i - 1 + validUrls.length) % validUrls.length,
                    )
                  }
                  sx={navBtnSx}
                >
                  <ChevronLeftIcon sx={ctrlIconSx} />
                </IconButton>
                <Box sx={photoBadgeSx}>
                  <PhotoCameraOutlinedIcon
                    sx={{
                      fontSize: tk.iconSizes.control,
                      color: tk.fleetColors.textSecondary,
                    }}
                  />
                  <Typography
                    variant="caption"
                    sx={{ color: 'inherit', fontWeight: 500 }}
                  >
                    {`${safeIdx + 1} / ${validUrls.length}`}
                  </Typography>
                </Box>
              </Box>
              <IconButton
                size="small"
                aria-label={`Next photo for ${site.name}`}
                onClick={() => setPhotoIdx((i) => (i + 1) % validUrls.length)}
                sx={navBtnSx}
              >
                <ChevronRightIcon sx={ctrlIconSx} />
              </IconButton>
            </Box>
          )}
        </Box>
      ) : (
        <Box
          data-testid={`site-photo-placeholder-${site.id}`}
          sx={carouselBoxSx}
        >
          <Box sx={placeholderStackSx}>
            <PhotoCameraOutlinedIcon sx={{ fontSize: tk.iconSizes.inline }} />
            <Typography variant="caption" color="text.secondary">
              No facility photo
            </Typography>
          </Box>
        </Box>
      )}

      <Box sx={cardBodySx}>
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
            <Box sx={{ ...actionRowSx, gap: 0.25, flexShrink: 0 }}>
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

        <Box sx={footerSx}>
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
                    <OpenInNewIcon sx={{ fontSize: tk.fontSizes.caption }} />
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
    </Box>
  );
};
