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

import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import CheckIcon from '@mui/icons-material/Check';
import CloudUploadOutlinedIcon from '@mui/icons-material/CloudUploadOutlined';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import PhotoLibraryOutlinedIcon from '@mui/icons-material/PhotoLibraryOutlined';
import StarIcon from '@mui/icons-material/Star';
import StarBorderIcon from '@mui/icons-material/StarBorder';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Link,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { ChangeEvent, useRef, useState } from 'react';

import { tonalPanelSx } from '@/fleet/theme/recipes';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';

import {
  arePhotoListsEqual,
  BULK_PHOTO_DROPBOX_URL,
  CAPTAIN_FIN_CHAT_URL,
  COSMETIC_DROPBOX_GO_LINK,
  MAX_PHOTOS_PER_ENTRY,
  buildBulkDropboxSyncPrompt,
  buildJetskiPrefilledPromptUrl,
} from './catalog_edit_validation';
import { CategoryIllustration } from './product_catalogue_card_view';

export interface CatalogDiffItem {
  readonly label: string;
  readonly before: string;
  readonly after: string;
}

export interface CatalogPhotoEditDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onCopyPrompt: () => void;
  readonly copiedPrompt?: boolean;
  readonly prompt?: string;
  readonly productCatalogId: string;
  readonly productName?: string;
  readonly resolvedProductType: string;
  readonly originalPhotos: readonly string[];
  readonly stagedPhotos: readonly string[];
  readonly photoUrlDraft: string;
  readonly photoInputError?: string;
  readonly photoDimensionErrors: Readonly<Record<string, string>>;
  readonly onPhotoUrlDraftChange: (value: string) => void;
  readonly onStagePhotoUrl: () => void;
  readonly onStageUploadedDataUrl: (dataUrl: string) => void;
  readonly onRemoveStagedPhoto: (index: number) => void;
  readonly onSetPrimaryPhoto: (index: number) => void;
  readonly onResetPhotos: () => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

const formatPhotoUriLabel = (uri: string, index: number) => {
  if (uri.startsWith('data:image/')) {
    return `Photo ${index + 1}`;
  }
  return uri.length > 44 ? `${uri.slice(0, 41)}...` : uri;
};

/**
 * Modal dialog for editing, uploading, and previewing Product Catalog reference
 * photos in a 350x184 card-aspect preview frame. Clicking "Copy prompt to save"
 * copies the prompt directly to the clipboard and surfaces a concise inline
 * confirmation notice with links to `@Captain Fin` and Jetski.
 */
export const CatalogPhotoEditDialog = ({
  open,
  onClose,
  onCopyPrompt,
  copiedPrompt = false,
  prompt = '',
  productCatalogId,
  productName,
  resolvedProductType,
  originalPhotos,
  stagedPhotos,
  photoUrlDraft,
  photoInputError,
  photoDimensionErrors,
  onPhotoUrlDraftChange,
  onStagePhotoUrl,
  onStageUploadedDataUrl,
  onRemoveStagedPhoto,
  onSetPrimaryPhoto,
  onResetPhotos,
  onPhotoDimensionsEvaluated,
}: CatalogPhotoEditDialogProps) => {
  const [selectedIdx, setSelectedIdx] = useState(0);
  const [copiedBulkPrompt, setCopiedBulkPrompt] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const safeIdx =
    stagedPhotos.length > 0
      ? Math.min(selectedIdx, stagedPhotos.length - 1)
      : 0;
  const activePreviewUri = stagedPhotos[safeIdx];
  const activePreviewDimError =
    activePreviewUri && !originalPhotos.includes(activePreviewUri)
      ? photoDimensionErrors[activePreviewUri]
      : undefined;
  const hasPhotoChanges = !arePhotoListsEqual(originalPhotos, stagedPhotos);

  const activeDimErrors = stagedPhotos
    .filter(
      (uri) =>
        Boolean(photoDimensionErrors[uri]) && !originalPhotos.includes(uri),
    )
    .map((uri) => photoDimensionErrors[uri]);

  const handleCopyBulkDropboxPrompt = async () => {
    const bulkPrompt = buildBulkDropboxSyncPrompt(productCatalogId);
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(bulkPrompt);
      } else if (typeof document !== 'undefined' && document.execCommand) {
        document.execCommand('copy');
      }
      setCopiedBulkPrompt(true);
      window.setTimeout(() => setCopiedBulkPrompt(false), 2500);
    } catch {
      setCopiedBulkPrompt(false);
    }
  };

  const handleStageUrlAndSelect = () => {
    const nextIdx = stagedPhotos.length;
    onStagePhotoUrl();
    if (photoUrlDraft.trim() && nextIdx < MAX_PHOTOS_PER_ENTRY) {
      setSelectedIdx(nextIdx);
    }
  };

  const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (
      !file ||
      stagedPhotos.length >= MAX_PHOTOS_PER_ENTRY ||
      file.size > 10 * 1024 * 1024
    ) {
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        onStageUploadedDataUrl(reader.result);
        setSelectedIdx(stagedPhotos.length);
      }
    };
    reader.readAsDataURL(file);
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      data-testid="catalog-photo-editor-panel"
      slotProps={{
        paper: {
          sx: {
            borderRadius: radii.md,
            border: borders.default,
          },
        },
      }}
    >
      <DialogTitle
        sx={{
          py: 1.5,
          px: 2.5,
          borderBottom: borders.default,
          display: 'flex',
          alignItems: 'center',
          gap: 1,
        }}
      >
        <PhotoLibraryOutlinedIcon
          sx={{ color: fleetColors.primary, fontSize: iconSizes.logo }}
        />
        <Typography
          variant="h6"
          component="span"
          sx={{
            fontFamily: fontFamilies.display,
            fontSize: fontSizes.title,
            fontWeight: 700,
            color: fleetColors.textPrimary,
          }}
        >
          Edit reference photos for {productCatalogId}
        </Typography>
      </DialogTitle>

      <DialogContent
        sx={{
          pt: 2,
          pb: 2,
          px: 2.5,
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
        }}
      >
        {copiedPrompt && (
          <Alert severity="success" data-testid="handoff-guidance">
            <Typography variant="body2" sx={{ fontSize: fontSizes.label }}>
              <strong>Copied prompt for {productCatalogId}!</strong> Paste into{' '}
              <Link
                href={CAPTAIN_FIN_CHAT_URL}
                target="_blank"
                rel="noopener noreferrer"
                data-testid="open-captain-fin-btn"
                sx={{ fontWeight: 600 }}
              >
                @Captain Fin
              </Link>{' '}
              or{' '}
              <Link
                href={buildJetskiPrefilledPromptUrl(prompt)}
                target="_blank"
                rel="noopener noreferrer"
                data-testid="open-in-jetski-btn"
                sx={{ fontWeight: 600 }}
              >
                Jetski
              </Link>{' '}
              to create the CL.
            </Typography>
          </Alert>
        )}

        {copiedPrompt && prompt && (
          <Box
            component="pre"
            data-testid="generated-catalog-prompt"
            sx={{ display: 'none' }}
          >
            {prompt}
          </Box>
        )}

        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: '360px 1fr' },
            gap: 2.5,
            alignItems: 'start',
          }}
        >
          {/* Left Column: 350x184 Card Preview & Staged Thumbnails */}
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.25 }}>
            <Typography
              variant="caption"
              sx={{
                fontSize: fontSizes.caption,
                fontWeight: 600,
                color: fleetColors.textSecondary,
              }}
            >
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
              {activePreviewUri ? (
                <Box
                  component="img"
                  src={activePreviewUri}
                  alt={`${productName || productCatalogId} staged preview`}
                  onLoad={(e) => {
                    const img = e.currentTarget as HTMLImageElement;
                    if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                      onPhotoDimensionsEvaluated(
                        activePreviewUri,
                        img.naturalWidth,
                        img.naturalHeight,
                      );
                    }
                  }}
                  onError={() =>
                    onPhotoDimensionsEvaluated(activePreviewUri, 0, 0)
                  }
                  sx={{
                    width: '100%',
                    height: '100%',
                    objectFit: 'cover',
                    objectPosition: 'center',
                    display: 'block',
                  }}
                />
              ) : (
                <CategoryIllustration
                  productType={resolvedProductType}
                  productCatalogId={productCatalogId}
                />
              )}
            </Box>

            {activePreviewDimError && (
              <Alert severity="error" sx={{ py: 0.5 }}>
                <Typography
                  variant="caption"
                  sx={{ fontSize: fontSizes.caption }}
                >
                  {activePreviewDimError}
                </Typography>
              </Alert>
            )}

            {/* Staged Thumbnail Strip with Primary Star & Delete */}
            <Box>
              <Typography
                variant="caption"
                sx={{
                  display: 'block',
                  fontSize: fontSizes.caption,
                  fontWeight: 600,
                  color: fleetColors.textSecondary,
                  mb: 0.75,
                }}
              >
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
                        onClick={() => setSelectedIdx(idx)}
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
                        <Box
                          component="img"
                          src={uri}
                          alt={formatPhotoUriLabel(uri, idx)}
                          onLoad={(e) => {
                            const img = e.currentTarget as HTMLImageElement;
                            if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                              onPhotoDimensionsEvaluated(
                                uri,
                                img.naturalWidth,
                                img.naturalHeight,
                              );
                            }
                          }}
                          onError={() => onPhotoDimensionsEvaluated(uri, 0, 0)}
                          sx={{
                            width: '100%',
                            height: '100%',
                            objectFit: 'cover',
                            display: 'block',
                          }}
                        />
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
                                setSelectedIdx(0);
                              }}
                              data-testid={`set-primary-photo-${idx}`}
                              sx={{
                                position: 'absolute',
                                top: 2,
                                left: 2,
                                p: 0.2,
                                backgroundColor: fleetColors.surface,
                                color:
                                  idx === 0
                                    ? fleetColors.primary
                                    : fleetColors.textSecondary,
                                '&:hover': {
                                  backgroundColor: fleetColors.surfaceTonal,
                                },
                                '&.Mui-disabled': {
                                  backgroundColor: fleetColors.surface,
                                  color: fleetColors.primary,
                                },
                              }}
                            >
                              {idx === 0 ? (
                                <StarIcon
                                  sx={{ fontSize: iconSizes.control }}
                                />
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
                            sx={{
                              position: 'absolute',
                              top: 2,
                              right: 2,
                              p: 0.2,
                              backgroundColor: fleetColors.surface,
                              color: fleetColors.textSecondary,
                              '&:hover': {
                                backgroundColor: statusTones.error.bg,
                                color: statusTones.error.fg,
                              },
                            }}
                          >
                            <DeleteOutlineIcon
                              sx={{ fontSize: iconSizes.control }}
                            />
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

          {/* Right Column: Single Consolidated Photo Input & Drop-Box Panel */}
          <Box
            sx={{
              ...tonalPanelSx,
              p: 2,
              display: 'flex',
              flexDirection: 'column',
              gap: 1.5,
            }}
          >
            <Typography
              variant="subtitle2"
              sx={{
                fontSize: fontSizes.label,
                fontWeight: 600,
                color: fleetColors.textPrimary,
              }}
            >
              Add photo
            </Typography>

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <TextField
                size="small"
                fullWidth
                placeholder="Paste image URL (http://screen/... or https://...)"
                value={photoUrlDraft}
                error={Boolean(photoInputError)}
                onChange={(e) => onPhotoUrlDraftChange(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleStageUrlAndSelect();
                  }
                }}
                sx={{
                  '& .MuiInputBase-root': {
                    height: controlHeights.button,
                    fontSize: fontSizes.label,
                    backgroundColor: fleetColors.surface,
                  },
                }}
                slotProps={{
                  htmlInput: {
                    'data-testid': 'photo-url-input',
                  },
                }}
              />
              <Button
                variant="outlined"
                size="small"
                onClick={handleStageUrlAndSelect}
                data-testid="add-photo-url-btn"
                sx={{
                  minHeight: controlHeights.button,
                  borderRadius: radii.pill,
                  fontSize: fontSizes.label,
                  textTransform: 'none',
                  px: 2,
                  whiteSpace: 'nowrap',
                  backgroundColor: fleetColors.surface,
                }}
              >
                Add
              </Button>
            </Box>

            {photoInputError && (
              <Typography
                variant="caption"
                sx={{
                  fontSize: fontSizes.caption,
                  color: statusTones.error.fg,
                }}
              >
                {photoInputError}
              </Typography>
            )}

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                disabled={stagedPhotos.length >= MAX_PHOTOS_PER_ENTRY}
                onChange={handleFileChange}
                style={{ display: 'none' }}
                data-testid="upload-photo-file-input"
              />
              <Button
                variant="outlined"
                size="small"
                startIcon={
                  <CloudUploadOutlinedIcon
                    sx={{ fontSize: iconSizes.control }}
                  />
                }
                disabled={stagedPhotos.length >= MAX_PHOTOS_PER_ENTRY}
                onClick={() => fileInputRef.current?.click()}
                data-testid="upload-photo-file-btn"
                sx={{
                  minHeight: controlHeights.chipCompact,
                  borderRadius: radii.pill,
                  fontSize: fontSizes.caption,
                  textTransform: 'none',
                  borderColor: fleetColors.border,
                  color: fleetColors.textPrimary,
                  backgroundColor: fleetColors.surface,
                }}
              >
                Upload image from computer
              </Button>
            </Box>

            <Box
              sx={{
                pt: 1.5,
                borderTop: borders.divider,
                display: 'flex',
                flexDirection: 'column',
                gap: 0.75,
              }}
            >
              <Typography
                variant="caption"
                data-testid="bulk-photo-dropbox-note"
                sx={{
                  fontSize: fontSizes.caption,
                  color: fleetColors.textSecondary,
                  lineHeight: 1.4,
                }}
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
                  onClick={handleCopyBulkDropboxPrompt}
                  startIcon={
                    copiedBulkPrompt ? (
                      <CheckIcon sx={{ fontSize: iconSizes.control }} />
                    ) : (
                      <ContentCopyIcon sx={{ fontSize: iconSizes.control }} />
                    )
                  }
                  data-testid="copy-bulk-dropbox-prompt-btn"
                  sx={{
                    minHeight: controlHeights.chipCompact,
                    py: 0.25,
                    px: 1,
                    borderRadius: radii.pill,
                    fontSize: fontSizes.caption,
                    textTransform: 'none',
                  }}
                >
                  {copiedBulkPrompt
                    ? 'Copied drop-box sync prompt!'
                    : 'Copy drop-box sync prompt'}
                </Button>
              </Box>
            </Box>
          </Box>
        </Box>
      </DialogContent>

      <DialogActions
        sx={{
          px: 2.5,
          py: 1.5,
          borderTop: borders.default,
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 1,
        }}
      >
        <Box>
          {hasPhotoChanges && (
            <Button
              variant="text"
              size="small"
              onClick={onResetPhotos}
              data-testid="reset-staged-photos-btn"
              sx={{
                minHeight: controlHeights.button,
                borderRadius: radii.pill,
                fontSize: fontSizes.label,
                textTransform: 'none',
                color: fleetColors.textSecondary,
              }}
            >
              Reset photos
            </Button>
          )}
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Button
            variant="outlined"
            size="small"
            onClick={onClose}
            data-testid="keep-editing-photos-btn"
            sx={{
              minHeight: controlHeights.button,
              borderRadius: radii.pill,
              fontSize: fontSizes.label,
              textTransform: 'none',
              borderColor: fleetColors.border,
              color: fleetColors.textPrimary,
            }}
          >
            Keep editing
          </Button>
          <Button
            variant="contained"
            size="small"
            disabled={activeDimErrors.length > 0 || !hasPhotoChanges}
            onClick={onCopyPrompt}
            startIcon={
              copiedPrompt ? (
                <CheckIcon sx={{ fontSize: iconSizes.control }} />
              ) : (
                <AutoAwesomeIcon sx={{ fontSize: iconSizes.control }} />
              )
            }
            data-testid="save-photos-by-prompt-btn"
            sx={{
              minHeight: controlHeights.button,
              borderRadius: radii.pill,
              fontSize: fontSizes.label,
              textTransform: 'none',
              backgroundColor: copiedPrompt
                ? statusTones.success.fg
                : fleetColors.primary,
              '&:hover': {
                backgroundColor: copiedPrompt
                  ? statusTones.success.fg
                  : fleetColors.primaryHover,
              },
            }}
          >
            {copiedPrompt ? 'Copied prompt!' : 'Copy prompt to save'}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
};
