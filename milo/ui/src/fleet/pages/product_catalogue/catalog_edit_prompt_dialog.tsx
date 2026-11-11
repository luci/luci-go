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
import PhotoLibraryOutlinedIcon from '@mui/icons-material/PhotoLibraryOutlined';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Link,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { actionRowSx, tonalPanelSx } from '@/fleet/theme/recipes';
import {
  borders,
  controlHeights,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';

import {
  arePhotoListsEqual,
  CAPTAIN_FIN_CHAT_URL,
  MAX_PHOTOS_PER_ENTRY,
  buildJetskiPrefilledPromptUrl,
} from './catalog_edit_validation';
import {
  CatalogBulkDropboxFooter,
  CatalogPhotoGalleryEditor,
} from './catalog_photo_gallery_editor';

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
  readonly onStagePhotoUrl: () => boolean;
  readonly onRemoveStagedPhoto: (index: number) => void;
  readonly onSetPrimaryPhoto: (index: number) => void;
  readonly onResetPhotos: () => void;
  readonly onPhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

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
  onRemoveStagedPhoto,
  onSetPrimaryPhoto,
  onResetPhotos,
  onPhotoDimensionsEvaluated,
}: CatalogPhotoEditDialogProps) => {
  const [selectedIdx, setSelectedIdx] = useState(0);
  const hasPhotoChanges = !arePhotoListsEqual(originalPhotos, stagedPhotos);

  const activeDimErrors = stagedPhotos
    .filter(
      (uri) =>
        Boolean(photoDimensionErrors[uri]) && !originalPhotos.includes(uri),
    )
    .map((uri) => photoDimensionErrors[uri]);

  const handleStageUrlAndSelect = () => {
    const nextIdx = stagedPhotos.length;
    if (onStagePhotoUrl() && nextIdx < MAX_PHOTOS_PER_ENTRY) {
      setSelectedIdx(nextIdx);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      data-testid="catalog-photo-editor-panel"
      slotProps={{
        paper: { sx: { borderRadius: radii.md, border: borders.default } },
      }}
    >
      <DialogTitle
        sx={{
          py: 1.5,
          px: 2.5,
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
        dividers
        sx={{
          py: 2,
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
          <CatalogPhotoGalleryEditor
            productCatalogId={productCatalogId}
            productName={productName}
            resolvedProductType={resolvedProductType}
            originalPhotos={originalPhotos}
            stagedPhotos={stagedPhotos}
            selectedIdx={selectedIdx}
            onSelectIdx={setSelectedIdx}
            photoDimensionErrors={photoDimensionErrors}
            onRemoveStagedPhoto={onRemoveStagedPhoto}
            onSetPrimaryPhoto={onSetPrimaryPhoto}
            onPhotoDimensionsEvaluated={onPhotoDimensionsEvaluated}
          />

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
                sx={{ backgroundColor: fleetColors.surface }}
                slotProps={{ htmlInput: { 'data-testid': 'photo-url-input' } }}
              />
              <Button
                variant="outlined"
                size="small"
                onClick={handleStageUrlAndSelect}
                data-testid="add-photo-url-btn"
                sx={{ height: controlHeights.input, px: 2, flexShrink: 0 }}
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

            <CatalogBulkDropboxFooter productCatalogId={productCatalogId} />
          </Box>
        </Box>
      </DialogContent>

      <DialogActions
        sx={{
          px: 2.5,
          py: 1.5,
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
              color="inherit"
              onClick={() => {
                onResetPhotos();
                setSelectedIdx(0);
              }}
              data-testid="reset-staged-photos-btn"
            >
              Reset photos
            </Button>
          )}
        </Box>

        <Box sx={actionRowSx}>
          <Button
            variant="outlined"
            size="small"
            onClick={onClose}
            data-testid="keep-editing-photos-btn"
          >
            Close
          </Button>
          <Button
            variant="contained"
            size="small"
            color={copiedPrompt ? 'success' : 'primary'}
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
          >
            {copiedPrompt ? 'Copied prompt!' : 'Copy prompt to save'}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
};
