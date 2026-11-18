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
  readonly mode?: 'site' | 'photos';
  readonly isSaving: boolean;
  readonly error: unknown;
  readonly onClose: () => void;
  readonly onSave: (site: PhysicalSite) => void;
}

export interface SitePhotosDialogProps {
  readonly site: PhysicalSite | null;
  readonly isSaving: boolean;
  readonly error: unknown;
  readonly onClose: () => void;
  readonly onSave: (site: PhysicalSite) => void;
}

const monoSlotProps = {
  htmlInput: { style: { fontFamily: tk.fontFamilies.mono } },
} as const;
const zeroTimeout = { timeout: 0 } as const;
const ctrlIconSx = { fontSize: tk.iconSizes.control } as const;
const dialogPaperProps = { sx: { maxWidth: 760 } } as const;
const photosDialogPaperProps = { sx: { maxWidth: 640 } } as const;
const colFlex = { display: 'flex', flexDirection: 'column' } as const;
const contentSx = {
  ...colFlex,
  gap: 2.25,
  maxHeight: '55vh',
  '&&': { pt: 1.5 },
} as const;
const topFieldsGridSx = {
  display: 'grid',
  gridTemplateColumns: { xs: '1fr', sm: '240px 1fr' },
  gap: 2,
} as const;
const photoGridSx = {
  display: 'grid',
  gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr' },
  gap: 1.25,
  mt: 0.5,
} as const;
const photoRowSx = {
  display: 'grid',
  gridTemplateColumns: 'minmax(0, 1fr) auto',
  alignItems: 'center',
  gap: 0.75,
  p: 1,
  borderRadius: tk.radii.md,
  border: tk.borders.default,
  backgroundColor: tk.fleetColors.surfaceTonal,
} as const;
const photoThumbSx = {
  gridColumn: '1 / -1',
  width: '100%',
  height: 108,
  objectFit: 'cover',
  borderRadius: tk.radii.sm,
  border: tk.borders.default,
  backgroundColor: tk.fleetColors.surface,
} as const;

export const SiteFormDialog = (p: SiteFormDialogProps) => {
  const { open, editingSite, existingSites, isSaving, error, onClose, onSave } =
    p;
  const mode = p.mode ?? 'site';
  const isPhotosMode = mode === 'photos';
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
  const isInvalidPhoto = trimmedPhoto
    .split(/[\n,]+/)
    .some((t) => t.trim() && !normalizeSiteImageUrl(t));
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
  const validSite = Boolean(
    trimmedId && trimmedName && !isDuplicateId && !isDuplicateName,
  );
  const canSave =
    !isSaving && (isPhotosMode ? isEditing && !isInvalidPhoto : validSite);

  const handleAddPhoto = () => {
    if (isInvalidPhoto) return;
    const parsed = parseSiteImageUrlsInput(photoInput);
    if (parsed.length === 0) return;
    setImageUrls((prev) => Array.from(new Set([...prev, ...parsed])));
    setPhotoInput('');
  };

  const handleSave = () => {
    if (!canSave) return;
    const base = {
      ...editingSite,
      imageUrls: Array.from(
        new Set([...imageUrls, ...parseSiteImageUrlsInput(photoInput)]),
      ),
    };
    onSave(
      PhysicalSite.fromPartial(
        isPhotosMode
          ? base
          : {
              ...base,
              id: isEditing ? editingSite!.id : trimmedId,
              name: trimmedName,
              platformFilters: buildPlatformFilters(filterByPlatform),
            },
      ),
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

  const dialogSections = {
    site: (
      <>
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
      </>
    ),
    photos: (
      <>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Box sx={{ ...actionRowSx, justifyContent: 'space-between', gap: 1 }}>
            <Typography variant="caption" color="text.secondary">
              Paste a SnipIt link (e.g. screen/ID) or image URL below.
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
              startIcon={<AddPhotoAlternateOutlinedIcon sx={ctrlIconSx} />}
              onClick={handleAddPhoto}
              disabled={!trimmedPhoto || isInvalidPhoto || isSaving}
              sx={{ flexShrink: 0, height: 40 }}
            >
              Add photo
            </Button>
          </Box>
          {imageUrls.length > 0 && (
            <Box sx={photoGridSx}>
              {imageUrls.map((url) => {
                const label = getSnipItShortlinkLabel(url);
                return (
                  <Box key={url} sx={photoRowSx}>
                    <Box component="img" src={url} alt="" sx={photoThumbSx} />
                    <Typography
                      variant="caption"
                      noWrap
                      sx={{ fontFamily: tk.fontFamilies.mono }}
                    >
                      {label}
                    </Typography>
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
                        <DeleteOutlineIcon sx={ctrlIconSx} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                );
              })}
            </Box>
          )}
        </Box>
      </>
    ),
  };

  const errVerb = isPhotosMode
    ? 'update facility photos'
    : `${isEditing ? 'update' : 'create'} physical site`;
  const saveLabel = isPhotosMode
    ? 'Save photos'
    : isEditing
      ? 'Save changes'
      : 'Add site';

  return (
    <Dialog
      open={open}
      onClose={isSaving ? undefined : onClose}
      maxWidth={isPhotosMode ? 'sm' : 'md'}
      fullWidth
      PaperProps={isPhotosMode ? photosDialogPaperProps : dialogPaperProps}
      TransitionProps={zeroTimeout}
    >
      <DialogTitle sx={{ pb: 1 }}>
        {isPhotosMode
          ? `Facility photos — ${editingSite?.name ?? ''}`
          : `${isEditing ? 'Edit' : 'Add'} physical site`}
      </DialogTitle>
      <DialogContent sx={contentSx}>
        {dialogSections[mode]}
        {Boolean(error) && (
          <Alert severity="error">{getErrorMessage(error, errVerb)}</Alert>
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
          {saveLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export const SitePhotosDialog = (p: SitePhotosDialogProps) => (
  <SiteFormDialog
    {...p}
    open={Boolean(p.site)}
    editingSite={p.site}
    existingSites={[]}
    mode="photos"
  />
);
