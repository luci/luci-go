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

import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import StarIcon from '@mui/icons-material/Star';
import StarBorderIcon from '@mui/icons-material/StarBorder';
import {
  Alert,
  Box,
  Button,
  IconButton,
  Link,
  Tooltip,
  Typography,
} from '@mui/material';
import { SyntheticEvent, useEffect, useState } from 'react';

import {
  borders,
  elevations,
  fleetColors,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';

import { copyTextToClipboard } from './catalog_details_helpers';
import {
  BULK_PHOTO_DROPBOX_URL,
  COSMETIC_DROPBOX_GO_LINK,
  buildBulkDropboxSyncPrompt,
  resolvePhotoPreviewSrc,
} from './catalog_edit_validation';
import { CategoryIllustration } from './category_illustration';
export interface CatalogPhotoGalleryEditorProps {
  readonly productCatalogId: string;
  readonly productName?: string;
  readonly resolvedProductType: string;
  readonly originalPhotos: readonly string[];
  readonly stagedPhotos: readonly string[];
  readonly selectedIdx: number;
  readonly onSelectIdx: (index: number) => void;
  readonly photoDimensionErrors: Readonly<Record<string, string>>;
  readonly onRemoveStagedPhoto: (index: number) => void;
  readonly onSetPrimaryPhoto: (index: number) => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

const formatPhotoUriLabel = (uri: string, index: number) =>
  uri.startsWith('data:image/')
    ? `Photo ${index + 1}`
    : uri.length > 44
      ? `${uri.slice(0, 41)}...`
      : uri;

const reportImgDimensions = (
  e: SyntheticEvent<HTMLImageElement>,
  uri: string,
  onEvaluated: (uri: string, width: number, height: number) => void,
) => {
  const { naturalWidth, naturalHeight } = e.currentTarget;
  if (naturalWidth > 0 && naturalHeight > 0) {
    onEvaluated(uri, naturalWidth, naturalHeight);
  }
};

const COVER_IMG_SX = {
  width: '100%',
  height: '100%',
  objectFit: 'cover',
  display: 'block',
} as const;

const SECTION_LABEL_SX = {
  display: 'block',
  fontSize: fontSizes.caption,
  fontWeight: 600,
  color: fleetColors.textSecondary,
} as const;

const getThumbBtnSx = (isRight: boolean, isPrimary = false) => ({
  position: 'absolute',
  top: 2,
  ...(isRight ? { right: 2 } : { left: 2 }),
  p: 0.2,
  backgroundColor: fleetColors.surface,
  color: isPrimary ? fleetColors.primary : fleetColors.textSecondary,
  '&:hover': isRight
    ? { backgroundColor: statusTones.error.bg, color: statusTones.error.fg }
    : { backgroundColor: fleetColors.surfaceTonal },
  '&.Mui-disabled': {
    backgroundColor: fleetColors.surface,
    color: fleetColors.primary,
  },
});

export const CatalogBulkDropboxFooter = ({
  productCatalogId,
}: {
  readonly productCatalogId: string;
}) => {
  const [copiedBulkPrompt, setCopiedBulkPrompt] = useState(false);
  useEffect(() => {
    if (!copiedBulkPrompt) return undefined;
    const t = setTimeout(() => setCopiedBulkPrompt(false), 2500);
    return () => clearTimeout(t);
  }, [copiedBulkPrompt]);
  return (
    <Box
      sx={{ pt: 1.5, borderTop: borders.divider, display: 'grid', gap: 0.75 }}
    >
      <Typography
        variant="caption"
        data-testid="bulk-photo-dropbox-note"
        sx={{ fontSize: fontSizes.caption, color: fleetColors.textSecondary }}
      >
        {'For bulk uploads, drop files named '}
        <code>{`${productCatalogId}.jpg`}</code>
        {' into '}
        <Link
          href={BULK_PHOTO_DROPBOX_URL}
          target="_blank"
          rel="noopener noreferrer"
          sx={{ fontWeight: 600 }}
        >
          {COSMETIC_DROPBOX_GO_LINK}
        </Link>
        {'.'}
      </Typography>
      <Box>
        <Button
          variant="text"
          size="small"
          onClick={async () =>
            setCopiedBulkPrompt(
              await copyTextToClipboard(
                buildBulkDropboxSyncPrompt(productCatalogId),
              ),
            )
          }
          startIcon={
            copiedBulkPrompt ? (
              <CheckIcon sx={{ fontSize: iconSizes.control }} />
            ) : (
              <ContentCopyIcon sx={{ fontSize: iconSizes.control }} />
            )
          }
          data-testid="copy-bulk-dropbox-prompt-btn"
        >
          {copiedBulkPrompt
            ? 'Copied drop-box sync prompt!'
            : 'Copy drop-box sync prompt'}
        </Button>
      </Box>
    </Box>
  );
};

export const CatalogPhotoGalleryEditor = ({
  productCatalogId,
  productName,
  resolvedProductType,
  originalPhotos,
  stagedPhotos,
  selectedIdx,
  onSelectIdx,
  photoDimensionErrors,
  onRemoveStagedPhoto,
  onSetPrimaryPhoto,
  onPhotoDimensionsEvaluated,
}: CatalogPhotoGalleryEditorProps) => {
  const [unpreviewableUris, setUnpreviewableUris] = useState<
    Record<string, boolean>
  >({});
  const markUriUnpreviewable = (uri: string) =>
    setUnpreviewableUris((p) => (p[uri] ? p : { ...p, [uri]: true }));

  const safeIdx =
    stagedPhotos.length > 0
      ? Math.min(selectedIdx, stagedPhotos.length - 1)
      : 0;
  const activePreviewUri = stagedPhotos[safeIdx];
  const activePreviewDimError =
    activePreviewUri && !originalPhotos.includes(activePreviewUri)
      ? photoDimensionErrors[activePreviewUri]
      : undefined;
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.25 }}>
      <Typography variant="caption" sx={SECTION_LABEL_SX}>
        Card preview
      </Typography>

      <Box
        data-testid="modal-photo-preview-frame"
        sx={{
          position: 'relative',
          width: '100%',
          height: 192,
          borderRadius: radii.md,
          overflow: 'hidden',
          border: activePreviewDimError
            ? `2px solid ${statusTones.error.fg}`
            : borders.default,
          background: `linear-gradient(135deg, ${fleetColors.surfaceTonal} 0%, ${fleetColors.primaryContainer} 100%)`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        {activePreviewUri && !unpreviewableUris[activePreviewUri] ? (
          <Box
            component="img"
            src={resolvePhotoPreviewSrc(activePreviewUri)}
            alt={`${productName || productCatalogId} staged preview`}
            data-testid="modal-photo-preview-img"
            onLoad={(e) =>
              reportImgDimensions(
                e,
                activePreviewUri,
                onPhotoDimensionsEvaluated,
              )
            }
            onError={() => markUriUnpreviewable(activePreviewUri)}
            sx={COVER_IMG_SX}
          />
        ) : (
          <CategoryIllustration
            productType={resolvedProductType}
            productCatalogId={productCatalogId}
          />
        )}
        {activePreviewUri && unpreviewableUris[activePreviewUri] && (
          <Typography
            variant="caption"
            noWrap
            title={activePreviewUri}
            data-testid="modal-photo-external-preview"
            sx={{
              position: 'absolute',
              inset: 'auto 8px 8px',
              px: 1,
              py: 0.5,
              borderRadius: radii.sm,
              backgroundColor: fleetColors.surface,
              border: borders.default,
              fontWeight: 600,
            }}
          >
            Staged URL: {formatPhotoUriLabel(activePreviewUri, safeIdx)}
          </Typography>
        )}
      </Box>

      {activePreviewDimError && (
        <Alert severity="error" sx={{ py: 0.5 }}>
          <Typography variant="caption" sx={{ fontSize: fontSizes.caption }}>
            {activePreviewDimError}
          </Typography>
        </Alert>
      )}

      <Box>
        <Typography variant="caption" sx={{ ...SECTION_LABEL_SX, mb: 0.75 }}>
          Staged photos
        </Typography>
        {stagedPhotos.length > 0 ? (
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              flexWrap: 'wrap',
            }}
          >
            {stagedPhotos.map((uri, idx) => {
              const dimError = !originalPhotos.includes(uri)
                ? photoDimensionErrors[uri]
                : undefined;
              return (
                <Box
                  key={`${uri}-${idx}`}
                  data-testid={`staged-photo-item-${idx}`}
                  onClick={() => onSelectIdx(idx)}
                  sx={{
                    position: 'relative',
                    width: 80,
                    height: 52,
                    borderRadius: radii.sm,
                    overflow: 'hidden',
                    cursor: 'pointer',
                    flexShrink: 0,
                    border: dimError
                      ? `2px solid ${statusTones.error.fg}`
                      : idx === safeIdx
                        ? `2px solid ${fleetColors.primary}`
                        : borders.default,
                    boxShadow: elevations.card,
                    backgroundColor: fleetColors.surfaceTonal,
                  }}
                >
                  {!unpreviewableUris[uri] && (
                    <Box
                      component="img"
                      src={resolvePhotoPreviewSrc(uri)}
                      alt={formatPhotoUriLabel(uri, idx)}
                      onLoad={(e) =>
                        reportImgDimensions(e, uri, onPhotoDimensionsEvaluated)
                      }
                      onError={() => markUriUnpreviewable(uri)}
                      sx={COVER_IMG_SX}
                    />
                  )}
                  <Tooltip
                    title={
                      idx === 0
                        ? 'Primary hero photo'
                        : `Set photo ${idx + 1} as primary`
                    }
                  >
                    <span tabIndex={idx === 0 ? 0 : undefined}>
                      <IconButton
                        size="small"
                        aria-label={
                          idx === 0
                            ? 'Primary photo'
                            : `Set photo ${idx + 1} as primary`
                        }
                        disabled={idx === 0}
                        onClick={(e) => {
                          e.stopPropagation();
                          onSetPrimaryPhoto(idx);
                          onSelectIdx(0);
                        }}
                        data-testid={`set-primary-photo-${idx}`}
                        sx={getThumbBtnSx(false, idx === 0)}
                      >
                        {idx === 0 ? (
                          <StarIcon sx={{ fontSize: iconSizes.control }} />
                        ) : (
                          <StarBorderIcon
                            sx={{ fontSize: iconSizes.control }}
                          />
                        )}
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title={`Remove photo ${idx + 1}`}>
                    <IconButton
                      size="small"
                      aria-label={`Remove photo ${idx + 1}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        onRemoveStagedPhoto(idx);
                      }}
                      data-testid={`remove-photo-${idx}`}
                      sx={getThumbBtnSx(true)}
                    >
                      <DeleteOutlineIcon sx={{ fontSize: iconSizes.control }} />
                    </IconButton>
                  </Tooltip>
                </Box>
              );
            })}
          </Box>
        ) : (
          <Typography
            variant="caption"
            sx={{
              fontSize: fontSizes.caption,
              color: fleetColors.textTertiary,
            }}
          >
            No reference photos staged.
          </Typography>
        )}
      </Box>
    </Box>
  );
};
