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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';

import { fontFamilies } from '@/fleet/theme/tokens';
import { getErrorMessage } from '@/fleet/utils/errors';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface SiteFormDialogProps {
  readonly open: boolean;
  readonly editingSite: PhysicalSite | null;
  readonly existingSites: readonly PhysicalSite[];
  readonly isSaving: boolean;
  readonly error: unknown;
  readonly onClose: () => void;
  readonly onSave: (site: PhysicalSite) => void;
}

export const SiteFormDialog = ({
  open,
  editingSite,
  existingSites,
  isSaving,
  error,
  onClose,
  onSave,
}: SiteFormDialogProps) => {
  const [siteId, setSiteId] = useState(editingSite?.id ?? '');
  const [siteName, setSiteName] = useState(editingSite?.name ?? '');

  const isEditing = Boolean(editingSite);
  const trimmedId = siteId.trim();
  const trimmedName = siteName.trim();

  const isDuplicateId = useMemo(() => {
    if (isEditing || !trimmedId) {
      return false;
    }
    const target = trimmedId.toLowerCase();
    return existingSites.some((s) => s.id.trim().toLowerCase() === target);
  }, [existingSites, isEditing, trimmedId]);

  const isDuplicateName = useMemo(() => {
    if (!trimmedName) {
      return false;
    }
    const target = trimmedName.toLowerCase();
    return existingSites.some(
      (s) => s.id !== editingSite?.id && s.name.trim().toLowerCase() === target,
    );
  }, [editingSite, existingSites, trimmedName]);

  const canSave =
    trimmedId !== '' &&
    trimmedName !== '' &&
    !isDuplicateId &&
    !isDuplicateName &&
    !isSaving;

  const handleSave = useCallback(() => {
    if (!canSave) {
      return;
    }
    onSave(
      PhysicalSite.fromPartial({
        id: isEditing ? editingSite!.id : trimmedId,
        name: trimmedName,
      }),
    );
  }, [canSave, editingSite, isEditing, onSave, trimmedId, trimmedName]);

  return (
    <Dialog
      open={open}
      onClose={isSaving ? undefined : onClose}
      maxWidth="xs"
      fullWidth
      TransitionProps={{ timeout: 0 }}
    >
      <DialogTitle sx={{ pb: 1 }}>
        {isEditing ? 'Edit physical site' : 'Add physical site'}
      </DialogTitle>
      <DialogContent
        onKeyDown={(e) => {
          if (e.key === 'Enter' && canSave) {
            e.preventDefault();
            handleSave();
          }
        }}
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          '&&': { pt: 1.5 },
        }}
      >
        <TextField
          label="Site code (ID)"
          required
          fullWidth
          size="small"
          placeholder="e.g. US-PAO-EM25"
          value={siteId}
          onChange={(e) => setSiteId(e.target.value)}
          disabled={isEditing || isSaving}
          error={isDuplicateId}
          helperText={
            isEditing
              ? 'Site code is the primary identifier and cannot be changed.'
              : isDuplicateId
                ? 'A site with this code already exists.'
                : 'Building or site code used in URLs (e.g. US-PAO-EM25, US-ATL-DAT3).'
          }
          slotProps={{
            htmlInput: {
              style: { fontFamily: fontFamilies.mono },
            },
          }}
        />
        <TextField
          label="Display name"
          required
          fullWidth
          size="small"
          placeholder="e.g. Palo Alto EM25"
          value={siteName}
          onChange={(e) => setSiteName(e.target.value)}
          disabled={isSaving}
          error={isDuplicateName}
          helperText={
            isDuplicateName
              ? 'A site with this display name already exists.'
              : 'Human-readable name shown across Fleet Console.'
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
