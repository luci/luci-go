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
import EditOutlinedIcon from '@mui/icons-material/EditOutlined';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';
import { useState } from 'react';

import { colors } from '@/fleet/theme/colors';
import { focusRingSx, iconButtonSx } from '@/fleet/theme/recipes';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  fontSizes,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';

import { CategoryIllustration } from './category_illustration';
import { UnifiedProductCatalogEntry } from './types';

export interface CatalogHeroPhotoStageProps {
  readonly entry: UnifiedProductCatalogEntry;
  readonly resolvedProductType: string;
  readonly isGce: boolean;
  readonly stagedPhotos: readonly string[];
  readonly onOpenPhotoEditor: () => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

const FLOATING_OVERLAY_ICON_BTN_SX = {
  ...iconButtonSx,
  backgroundColor: fleetColors.surface,
  border: borders.default,
  boxShadow: elevations.card,
  '&:hover': {
    backgroundColor: fleetColors.surface,
    borderColor: fleetColors.primary,
    color: fleetColors.primary,
  },
} as const;

const CAROUSEL_NAV_BTN_SX = {
  ...FLOATING_OVERLAY_ICON_BTN_SX,
  position: 'absolute',
  top: '50%',
  transform: 'translateY(-50%)',
  zIndex: 3,
  p: 0,
} as const;

const EDIT_PHOTO_BTN_SX = {
  ...FLOATING_OVERLAY_ICON_BTN_SX,
  position: 'absolute',
  top: 8,
  right: 8,
  zIndex: 5,
} as const;

export const CatalogHeroPhotoStage = ({
  entry,
  resolvedProductType,
  isGce,
  stagedPhotos,
  onOpenPhotoEditor,
  onPhotoDimensionsEvaluated,
}: CatalogHeroPhotoStageProps) => {
  const [activePhotoIdx, setActivePhotoIdx] = useState(0);
  const [failedUris, setFailedUris] = useState<Record<string, boolean>>({});

  const photos = stagedPhotos.filter((uri) => Boolean(uri) && !failedUris[uri]);
  const safePhotoIdx = photos.length > 0 ? activePhotoIdx % photos.length : 0;
  const currentPhotoUri = photos[safePhotoIdx];
  const hasPhoto = photos.length > 0;

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
        borderRight: { xs: 'none', md: borders.divider },
        borderBottom: { xs: borders.divider, md: 'none' },
        backgroundColor: fleetColors.surface,
      }}
    >
      <Box
        data-testid="details-photo-stage"
        sx={{
          position: 'relative',
          flex: 1,
          minHeight: 210,
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          p: 1.5,
          overflow: 'hidden',
          background: `linear-gradient(135deg, ${fleetColors.surfaceTonal} 0%, ${fleetColors.primaryContainer} 100%)`,
        }}
      >
        {!isGce && (
          <Tooltip title="Edit or upload reference photos">
            <IconButton
              size="small"
              aria-label="Edit photos"
              onClick={onOpenPhotoEditor}
              data-testid="toggle-photo-editor-btn"
              sx={EDIT_PHOTO_BTN_SX}
            >
              <EditOutlinedIcon sx={{ fontSize: iconSizes.control }} />
            </IconButton>
          </Tooltip>
        )}

        {hasPhoto ? (
          <>
            <Box
              component="img"
              src={currentPhotoUri}
              alt={`${entry.productName || entry.productCatalogId} hardware reference`}
              data-testid="details-photo-img"
              onLoad={(e) => {
                const img = e.currentTarget as HTMLImageElement;
                if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                  onPhotoDimensionsEvaluated(
                    currentPhotoUri,
                    img.naturalWidth,
                    img.naturalHeight,
                  );
                }
              }}
              onError={() =>
                setFailedUris((prev) => ({ ...prev, [currentPhotoUri]: true }))
              }
              sx={{
                position: 'absolute',
                inset: 0,
                width: '100%',
                height: '100%',
                objectFit: 'cover',
                objectPosition: 'center',
                display: 'block',
                zIndex: 0,
              }}
            />
            {photos.length > 1 && (
              <>
                <IconButton
                  aria-label="Previous photo"
                  onClick={() =>
                    setActivePhotoIdx(
                      (prev) => (prev - 1 + photos.length) % photos.length,
                    )
                  }
                  size="small"
                  sx={{ ...CAROUSEL_NAV_BTN_SX, left: 8 }}
                >
                  <ChevronLeftIcon
                    sx={{
                      fontSize: iconSizes.inline,
                      flexShrink: 0,
                      display: 'block',
                    }}
                  />
                </IconButton>
                <IconButton
                  aria-label="Next photo"
                  onClick={() =>
                    setActivePhotoIdx((prev) => (prev + 1) % photos.length)
                  }
                  size="small"
                  sx={{ ...CAROUSEL_NAV_BTN_SX, right: 8 }}
                >
                  <ChevronRightIcon
                    sx={{
                      fontSize: iconSizes.inline,
                      flexShrink: 0,
                      display: 'block',
                    }}
                  />
                </IconButton>
              </>
            )}
          </>
        ) : (
          <Box
            data-testid="details-photo-fallback"
            sx={{
              display: 'flex',
              flex: 1,
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <CategoryIllustration
              productType={resolvedProductType}
              productCatalogId={entry.productCatalogId}
            />
          </Box>
        )}

        {photos.length > 1 && (
          <Box
            data-testid="details-photo-thumbnails"
            sx={{
              position: 'absolute',
              left: 8,
              right: 8,
              bottom: 8,
              zIndex: 3,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 0.75,
              pointerEvents: 'none',
            }}
          >
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                pointerEvents: 'auto',
                overflowX: 'auto',
                maxWidth: 'calc(100% - 64px)',
              }}
            >
              {photos.map((uri, idx) => (
                <Box
                  key={`thumb-${idx}`}
                  component="button"
                  type="button"
                  aria-label={`View photo ${idx + 1} of ${photos.length}`}
                  onClick={() => setActivePhotoIdx(idx)}
                  sx={{
                    ...focusRingSx,
                    p: 0,
                    width: 40,
                    height: 28,
                    flexShrink: 0,
                    borderRadius: radii.sm,
                    overflow: 'hidden',
                    cursor: 'pointer',
                    border:
                      idx === safePhotoIdx
                        ? `2px solid ${fleetColors.primary}`
                        : borders.default,
                    boxShadow: elevations.card,
                    backgroundColor: fleetColors.surface,
                  }}
                >
                  <Box
                    component="img"
                    src={uri}
                    alt={`Thumbnail ${idx + 1}`}
                    sx={{
                      width: '100%',
                      height: '100%',
                      objectFit: 'cover',
                      display: 'block',
                    }}
                  />
                </Box>
              ))}
            </Box>
            <Box
              sx={{
                display: 'inline-flex',
                alignItems: 'center',
                px: 1,
                height: controlHeights.chipCompact,
                borderRadius: radii.pill,
                backgroundColor: colors.grey[900],
                color: fleetColors.surface,
                boxShadow: elevations.card,
              }}
            >
              <Typography
                variant="caption"
                sx={{
                  fontSize: fontSizes.caption,
                  fontWeight: 600,
                  lineHeight: 1,
                }}
              >
                {`${safePhotoIdx + 1} / ${photos.length}`}
              </Typography>
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  );
};
