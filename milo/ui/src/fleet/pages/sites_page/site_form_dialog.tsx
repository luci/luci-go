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

import AddPhotoAlternateOutlinedIcon from '@mui/icons-material/AddPhotoAlternateOutlined';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
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
import { KeyboardEvent, useState } from 'react';

import { actionRowSx, iconButtonSx } from '@/fleet/theme/recipes';
import * as tk from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  PhysicalSite,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  buildPlatformFilters,
  getSnipItShortlinkLabel,
  normalizeSiteImageUrl,
  parseSiteImageUrlsInput,
  platformFiltersToMap,
  SNIPIT_UPLOAD_URL,
} from './site_helpers';
import { SitePlatformFilterEditor } from './site_platform_rollup';

export interface SiteFormDialogProps {
  readonly open: boolean;
  readonly editingSite: PhysicalSite | null;
  readonly existingSites: readonly PhysicalSite[];
  readonly isSaving: boolean;
  readonly error: unknown;
  readonly onClose: () => void;
  readonly onSave: (site: PhysicalSite) => void;
}

const monoSlotProps = {
  htmlInput: { style: { fontFamily: tk.fontFamilies.mono } },
} as const;
const zeroTimeout = { timeout: 0 } as const;
const dialogPaperProps = { sx: { maxWidth: 760 } } as const;
const contentSx = {
  display: 'flex',
  flexDirection: 'column',
  gap: 2.25,
  maxHeight: '55vh',
  '&&': { pt: 1.5 },
} as const;
const topFieldsGridSx = {
  display: 'grid',
  gridTemplateColumns: { xs: '1fr', sm: '240px 1fr' },
  gap: 2,
  alignItems: 'flex-start',
} as const;
const photoRowSx = {
  ...actionRowSx,
  justifyContent: 'space-between',
  px: 1.25,
  py: 0.5,
  borderRadius: tk.radii.sm,
  border: tk.borders.divider,
  backgroundColor: tk.fleetColors.surfaceTonal,
} as const;
const photoThumbSx = {
  width: 36,
  height: 24,
  objectFit: 'cover',
  borderRadius: tk.radii.sm,
  border: tk.borders.default,
  backgroundColor: tk.fleetColors.surface,
  flexShrink: 0,
} as const;

export const SiteFormDialog = (p: SiteFormDialogProps) => {
  const { open, editingSite, existingSites, isSaving, error, onClose, onSave } =
    p;
  const [siteId, setSiteId] = useState(editingSite?.id ?? '');
  const [siteName, setSiteName] = useState(editingSite?.name ?? '');
  const [filterByPlatform, setFilterByPlatform] = useState<
    Partial<Record<Platform, string>>
  >(() => platformFiltersToMap(editingSite?.platformFilters));
  const [imageUrls, setImageUrls] = useState<string[]>(() =>
    Array.from(
      new Set(
        (editingSite?.imageUrls ?? [])
          .map(normalizeSiteImageUrl)
          .filter((u): u is string => u !== null),
      ),
    ),
  );
  const [photoInput, setPhotoInput] = useState('');

  const isEditing = Boolean(editingSite);
  const [trimmedId, trimmedName, trimmedPhoto] = [
    siteId.trim(),
    siteName.trim(),
    photoInput.trim(),
  ];
  const rawPhotoTokens = trimmedPhoto
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  const isInvalidPhoto = rawPhotoTokens.some(
    (token) => normalizeSiteImageUrl(token) === null,
  );
  const eqCI = (a: string, b: string) =>
    a.trim().toLowerCase() === b.toLowerCase();
  const isDuplicateId =
    !isEditing &&
    Boolean(trimmedId) &&
    existingSites.some((s) => eqCI(s.id, trimmedId));
  const isDuplicateName =
    Boolean(trimmedName) &&
    existingSites.some(
      (s) => s.id !== editingSite?.id && eqCI(s.name, trimmedName),
    );
  const canSave = Boolean(
    trimmedId &&
      trimmedName &&
      !isDuplicateId &&
      !isDuplicateName &&
      !isInvalidPhoto &&
      !isSaving,
  );

  const handleAddPhoto = () => {
    if (isInvalidPhoto) return;
    const parsed = parseSiteImageUrlsInput(photoInput);
    if (parsed.length === 0) return;
    setImageUrls((prev) => Array.from(new Set([...prev, ...parsed])));
    setPhotoInput('');
  };

  const handleSave = () => {
    if (!canSave) return;
    const finalPhotos = Array.from(
      new Set([...imageUrls, ...parseSiteImageUrlsInput(photoInput)]),
    );
    onSave(
      PhysicalSite.fromPartial({
        ...editingSite,
        id: isEditing ? editingSite!.id : trimmedId,
        name: trimmedName,
        platformFilters: buildPlatformFilters(filterByPlatform),
        imageUrls: finalPhotos,
      }),
    );
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' && canSave) {
      e.preventDefault();
      handleSave();
    }
  };

  const idHelper = isDuplicateId
    ? 'A site with this code already exists.'
    : isEditing
      ? undefined
      : 'Building code (e.g. US-PAO-EM25).';
  const nameHelper = isDuplicateName
    ? 'A site with this display name already exists.'
    : undefined;
  const tfBase = {
    required: true,
    fullWidth: true,
    size: 'small' as const,
    onKeyDown,
  };

  return (
    <Dialog
      open={open}
      onClose={isSaving ? undefined : onClose}
      maxWidth="md"
      fullWidth
      PaperProps={dialogPaperProps}
      TransitionProps={zeroTimeout}
    >
      <DialogTitle sx={{ pb: 1 }}>
        {isEditing ? 'Edit physical site' : 'Add physical site'}
      </DialogTitle>
      <DialogContent sx={contentSx}>
        <Box sx={topFieldsGridSx}>
          <TextField
            {...tfBase}
            label="Site code (ID)"
            placeholder="e.g. US-PAO-EM25"
            value={siteId}
            onChange={(e) => setSiteId(e.target.value)}
            disabled={isEditing || isSaving}
            error={isDuplicateId}
            helperText={idHelper}
            slotProps={monoSlotProps}
          />
          <TextField
            {...tfBase}
            label="Display name"
            placeholder="e.g. Palo Alto EM25"
            value={siteName}
            onChange={(e) => setSiteName(e.target.value)}
            disabled={isSaving}
            error={isDuplicateName}
            helperText={nameHelper}
          />
        </Box>
        <SitePlatformFilterEditor
          filterByPlatform={filterByPlatform}
          onChangeFilter={(plat, val) =>
            setFilterByPlatform((prev) => ({ ...prev, [plat]: val }))
          }
        />

        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          <Box sx={{ ...actionRowSx, justifyContent: 'space-between' }}>
            <Typography variant="subtitle2">
              Facility photos (SnipIt)
            </Typography>
            <Link
              href={SNIPIT_UPLOAD_URL}
              target="_blank"
              rel="noopener noreferrer"
              underline="hover"
              sx={{
                ...actionRowSx,
                gap: 0.5,
                fontSize: tk.fontSizes.caption,
                fontWeight: 500,
              }}
            >
              Open SnipIt upload page
              <OpenInNewIcon sx={{ fontSize: tk.fontSizes.caption }} />
            </Link>
          </Box>
          <Typography variant="caption" color="text.secondary">
            Upload photos on SnipIt, then paste each screenshot link (e.g.
            screen/ID) or image URL below.
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
            <TextField
              label="SnipIt screenshot link or URL"
              fullWidth
              size="small"
              placeholder="e.g. screen/ddNrsT8xfFq4nJa"
              value={photoInput}
              onChange={(e) => setPhotoInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && trimmedPhoto) {
                  e.preventDefault();
                  e.stopPropagation();
                  handleAddPhoto();
                }
              }}
              disabled={isSaving}
              error={isInvalidPhoto}
              helperText={
                isInvalidPhoto
                  ? 'Enter a SnipIt link (screen/ID) or https:// image URL.'
                  : undefined
              }
              slotProps={monoSlotProps}
            />
            <Button
              variant="toolbar"
              startIcon={
                <AddPhotoAlternateOutlinedIcon
                  sx={{ fontSize: tk.iconSizes.control }}
                />
              }
              onClick={handleAddPhoto}
              disabled={!trimmedPhoto || isInvalidPhoto || isSaving}
              sx={{ flexShrink: 0, height: 40 }}
            >
              Add photo
            </Button>
          </Box>
          {imageUrls.length > 0 && (
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: 0.75,
                mt: 0.5,
              }}
            >
              {imageUrls.map((url) => {
                const label = getSnipItShortlinkLabel(url);
                return (
                  <Box key={url} sx={photoRowSx}>
                    <Box sx={{ ...actionRowSx, gap: 1, minWidth: 0 }}>
                      <Box component="img" src={url} alt="" sx={photoThumbSx} />
                      <Typography
                        variant="caption"
                        noWrap
                        sx={{ fontFamily: tk.fontFamilies.mono }}
                      >
                        {label}
                      </Typography>
                    </Box>
                    <Tooltip title="Remove photo">
                      <IconButton
                        size="small"
                        aria-label={`Remove photo ${label}`}
                        onClick={() =>
                          setImageUrls((prev) => prev.filter((u) => u !== url))
                        }
                        disabled={isSaving}
                        sx={iconButtonSx}
                      >
                        <DeleteOutlineIcon
                          sx={{ fontSize: tk.iconSizes.control }}
                        />
                      </IconButton>
                    </Tooltip>
                  </Box>
                );
              })}
            </Box>
          )}
        </Box>

        {Boolean(error) && (
          <Alert severity="error">
            {getErrorMessage(
              error,
              isEditing ? 'update physical site' : 'create physical site',
            )}
          </Alert>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5, gap: 1 }}>
        <Button
          variant="toolbar"
          color="inherit"
          onClick={onClose}
          disabled={isSaving}
        >
          Cancel
        </Button>
        <Button variant="contained" onClick={handleSave} disabled={!canSave}>
          {isEditing ? 'Save changes' : 'Add site'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
