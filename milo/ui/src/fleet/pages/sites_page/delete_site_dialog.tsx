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
  Typography,
} from '@mui/material';

import { getErrorMessage } from '@/fleet/utils/errors';
import { PhysicalSite } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface DeleteSiteDialogProps {
  readonly site: PhysicalSite | null;
  readonly isDeleting: boolean;
  readonly error: unknown;
  readonly onClose: () => void;
  readonly onConfirmDelete: (id: string) => void;
}

export const DeleteSiteDialog = ({
  site,
  isDeleting,
  error,
  onClose,
  onConfirmDelete,
}: DeleteSiteDialogProps) => (
  <Dialog
    open={Boolean(site)}
    onClose={isDeleting ? undefined : onClose}
    maxWidth="xs"
    fullWidth
    TransitionProps={{ timeout: 0 }}
  >
    <DialogTitle sx={{ pb: 1 }}>Delete physical site</DialogTitle>
    <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
      <Typography variant="body2" color="text.secondary">
        {site
          ? `Delete "${site.name}" (${site.id})? This removes the site from the directory.`
          : ''}
      </Typography>
      {Boolean(error) && (
        <Alert severity="error">
          {getErrorMessage(error, 'delete physical site')}
        </Alert>
      )}
    </DialogContent>
    <DialogActions sx={{ px: 3, pb: 2.5, gap: 1 }}>
      <Button
        variant="toolbar"
        color="inherit"
        onClick={onClose}
        disabled={isDeleting}
      >
        Cancel
      </Button>
      <Button
        variant="contained"
        color="error"
        onClick={() => site && onConfirmDelete(site.id)}
        disabled={isDeleting || !site}
      >
        Delete site
      </Button>
    </DialogActions>
  </Dialog>
);
