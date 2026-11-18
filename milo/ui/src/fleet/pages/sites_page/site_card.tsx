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
import MoreVertIcon from '@mui/icons-material/MoreVert';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import PhotoCameraOutlinedIcon from '@mui/icons-material/PhotoCameraOutlined';
import PlaceOutlinedIcon from '@mui/icons-material/PlaceOutlined';
import {
  Box,
  IconButton,
  Link,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { HighlightCharacter } from '@/fleet/components/highlight_character';
import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  actionRowSx,
  iconButtonSx,
  surfaceCardClippedSx,
} from '@/fleet/theme/recipes';
import * as tk from '@/fleet/theme/tokens';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  formatSiteUpdatedAt,
  getCampusMapsUrl,
  normalizeSiteImageUrl,
} from './site_helpers';
import { SitePlatformRollup } from './site_platform_rollup';

export interface SiteCardProps {
  readonly site: PhysicalSite;
  readonly searchQuery?: string;
  readonly onEdit?: (site: PhysicalSite) => void;
  readonly onManagePhotos?: (site: PhysicalSite) => void;
  readonly onDelete?: (site: PhysicalSite) => void;
}

const ctrlIconSx = { fontSize: tk.iconSizes.control } as const;
const tipProps = { disableInteractive: true } as const;
const rowBetweenSx = {
  ...actionRowSx,
  justifyContent: 'space-between',
  flexWrap: 'nowrap',
  gap: 1,
} as const;
const colFlex = { display: 'flex', flexDirection: 'column' } as const;
const cardSx = { ...surfaceCardClippedSx, ...colFlex } as const;
const cardBodySx = { p: 2.25, ...colFlex, gap: 1.5, flex: 1 } as const;
const footerSx = { ...rowBetweenSx, mt: 'auto', pt: 1.5 } as const;
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
const photoPillSx = {
  position: 'absolute',
  bottom: 8,
  right: 8,
  ...actionRowSx,
  flexWrap: 'nowrap',
  gap: 0.25,
  px: 0.5,
  height: tk.controlHeights.chip,
  borderRadius: tk.radii.pill,
  backgroundColor: tk.fleetColors.scrim,
  color: tk.fleetColors.onScrim,
} as const;
const scrimBtnBaseSx = {
  p: 0,
  color: tk.fleetColors.onScrim,
  '&:hover': { backgroundColor: tk.fleetColors.scrimHover },
  '&:focus-visible': {
    outline: `2px solid ${tk.fleetColors.onScrim}`,
    outlineOffset: -2,
  },
} as const;
const photoNavBtnSx = {
  ...scrimBtnBaseSx,
  width: tk.controlHeights.chipCompact,
  height: tk.controlHeights.chipCompact,
} as const;
const photoCountSx = {
  color: tk.fleetColors.onScrim,
  fontWeight: 500,
  px: 0.5,
  fontVariantNumeric: 'tabular-nums',
  userSelect: 'none',
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
  ...colFlex,
  alignItems: 'center',
  gap: 0.5,
  color: tk.fleetColors.textTertiary,
} as const;
const photoEditBtnSx = {
  ...scrimBtnBaseSx,
  position: 'absolute',
  top: 8,
  right: 8,
  width: tk.controlHeights.button,
  height: tk.controlHeights.button,
  backgroundColor: tk.fleetColors.scrim,
} as const;

export const SiteCard = (props: SiteCardProps) => {
  const { site, searchQuery, onEdit, onManagePhotos, onDelete } = props;
  const [photoIdx, setPhotoIdx] = useState(0);
  const [failedUrls, setFailedUrls] = useState<Record<string, boolean>>({});
  const [menuAnchorEl, setMenuAnchorEl] = useState<HTMLElement | null>(null);
  const closeMenu = () => setMenuAnchorEl(null);
  const trimmedQuery = searchQuery?.trim() ?? '';
  const matchIdx = (t: string) =>
    trimmedQuery ? fuzzySubstring(trimmedQuery, t)[1] : undefined;
  const validUrls = [
    ...new Set(
      (site.imageUrls ?? [])
        .map(normalizeSiteImageUrl)
        .filter((u): u is string => u !== null && !failedUrls[u]),
    ),
  ];
  const safeIdx = validUrls.length > 0 ? photoIdx % validUrls.length : 0;
  const activeUrl = validUrls[safeIdx] ?? null;
  const campusMapsUrl = getCampusMapsUrl(site.id);
  const formattedUpdatedAt = formatSiteUpdatedAt(site.updatedAt);
  const stepPhoto = (d: number) =>
    setPhotoIdx((i) => (i + d + validUrls.length) % validUrls.length);
  const idBadge = (
    <HighlightCharacter
      variant="inherit"
      component="span"
      truncate="none"
      highlightVariant="background"
      highlightIndexes={matchIdx(site.id)}
      sx={{ display: 'inline', fontFamily: tk.fontFamilies.mono }}
    >
      {site.id}
    </HighlightCharacter>
  );

  return (
    <Box
      component="article"
      aria-label={site.name}
      data-testid={`site-card-${site.id}`}
      sx={cardSx}
    >
      {activeUrl ? (
        <Box data-testid={`site-photo-carousel-${site.id}`} sx={carouselBoxSx}>
          {validUrls.map((url, idx) => (
            <Box
              key={url}
              component="img"
              src={url}
              alt={`${site.name} facility photo ${idx + 1}`}
              onError={() =>
                setFailedUrls((prev) => ({ ...prev, [url]: true }))
              }
              sx={{
                width: 1,
                height: 1,
                objectFit: 'cover',
                display: idx === safeIdx ? 'block' : 'none',
              }}
            />
          ))}
          {onManagePhotos && (
            <Tooltip {...tipProps} title="Manage facility photos">
              <IconButton
                size="small"
                aria-label={`Manage photos for ${site.name}`}
                onClick={() => onManagePhotos(site)}
                sx={photoEditBtnSx}
              >
                <PhotoCameraOutlinedIcon
                  sx={{ fontSize: tk.fontSizes.caption }}
                />
              </IconButton>
            </Tooltip>
          )}
          {validUrls.length > 1 && (
            <Box sx={photoPillSx}>
              <IconButton
                size="small"
                aria-label={`Previous photo for ${site.name}`}
                onClick={() => stepPhoto(-1)}
                sx={photoNavBtnSx}
              >
                <ChevronLeftIcon sx={ctrlIconSx} />
              </IconButton>
              <Typography variant="caption" sx={photoCountSx}>
                {`${safeIdx + 1} / ${validUrls.length}`}
              </Typography>
              <IconButton
                size="small"
                aria-label={`Next photo for ${site.name}`}
                onClick={() => stepPhoto(1)}
                sx={photoNavBtnSx}
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
            {onManagePhotos && (
              <Link
                component="button"
                type="button"
                variant="caption"
                underline="hover"
                aria-label={`Manage photos for ${site.name}`}
                onClick={() => onManagePhotos(site)}
                sx={{ fontWeight: 500 }}
              >
                Add photos
              </Link>
            )}
          </Box>
        </Box>
      )}

      <Box sx={cardBodySx}>
        <Box sx={{ ...rowBetweenSx, minHeight: 32 }}>
          <Box sx={titleRowSx}>
            <PlaceOutlinedIcon sx={placeIconSx} />
            <Tooltip title={site.name} enterDelay={500}>
              <Box tabIndex={0} sx={{ minWidth: 0, flex: 1 }}>
                <HighlightCharacter
                  variant="subtitle1"
                  component="h2"
                  aria-label={site.name}
                  highlightVariant="background"
                  highlightIndexes={matchIdx(site.name)}
                  sx={{ fontWeight: 600 }}
                >
                  {site.name}
                </HighlightCharacter>
              </Box>
            </Tooltip>
          </Box>
          {(onEdit || onDelete) && (
            <Box sx={{ ...actionRowSx, gap: 0.25, flexShrink: 0 }}>
              {onEdit && (
                <Tooltip {...tipProps} title="Edit site">
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
                <>
                  <Tooltip
                    {...tipProps}
                    title={menuAnchorEl ? '' : 'More actions'}
                  >
                    <IconButton
                      size="small"
                      aria-label={`More actions for ${site.name}`}
                      aria-haspopup="menu"
                      aria-expanded={Boolean(menuAnchorEl)}
                      onClick={(e) => setMenuAnchorEl(e.currentTarget)}
                      sx={iconButtonSx}
                    >
                      <MoreVertIcon sx={ctrlIconSx} />
                    </IconButton>
                  </Tooltip>
                  <Menu
                    anchorEl={menuAnchorEl}
                    open={Boolean(menuAnchorEl)}
                    onClose={closeMenu}
                    anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                    transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                  >
                    <MenuItem
                      aria-label={`Delete ${site.name}`}
                      onClick={() => {
                        closeMenu();
                        onDelete(site);
                      }}
                      sx={{ color: 'error.main' }}
                    >
                      <ListItemIcon>
                        <DeleteOutlineIcon fontSize="small" color="error" />
                      </ListItemIcon>
                      <ListItemText disableTypography>Delete site</ListItemText>
                    </MenuItem>
                  </Menu>
                </>
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
                    {idBadge}
                    <OpenInNewIcon sx={{ fontSize: tk.fontSizes.caption }} />
                  </Box>
                </StatusPill>
              </Link>
            </Tooltip>
          ) : (
            <StatusPill tone="neutral" dot={false}>
              {idBadge}
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
