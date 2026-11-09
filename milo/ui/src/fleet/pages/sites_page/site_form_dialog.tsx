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

import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { KeyboardEvent, useState } from 'react';

import { fontFamilies } from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import {
  PhysicalSite,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { buildPlatformFilters, platformFiltersToMap } from './site_helpers';
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
  htmlInput: { style: { fontFamily: fontFamilies.mono } },
} as const;
const zeroTimeout = { timeout: 0 } as const;
const dialogPaperProps = { sx: { maxWidth: 760 } } as const;
const contentSx = {
  display: 'flex',
  flexDirection: 'column',
  gap: 2.25,
  '&&': { pt: 1.5 },
} as const;
const topFieldsGridSx = {
  display: 'grid',
  gridTemplateColumns: { xs: '1fr', sm: '240px 1fr' },
  gap: 2,
  alignItems: 'flex-start',
} as const;

export const SiteFormDialog = (p: SiteFormDialogProps) => {
  const { open, editingSite, existingSites, isSaving, error, onClose, onSave } =
    p;
  const [siteId, setSiteId] = useState(editingSite?.id ?? '');
  const [siteName, setSiteName] = useState(editingSite?.name ?? '');
  const [filterByPlatform, setFilterByPlatform] = useState<
    Partial<Record<Platform, string>>
  >(() => platformFiltersToMap(editingSite?.platformFilters));

  const isEditing = Boolean(editingSite);
  const [trimmedId, trimmedName] = [siteId.trim(), siteName.trim()];
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
    trimmedId && trimmedName && !isDuplicateId && !isDuplicateName && !isSaving,
  );

  const handleSave = () => {
    if (!canSave) return;
    onSave(
      PhysicalSite.fromPartial({
        ...editingSite,
        id: isEditing ? editingSite!.id : trimmedId,
        name: trimmedName,
        platformFilters: buildPlatformFilters(filterByPlatform),
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
