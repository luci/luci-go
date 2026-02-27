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

import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';

import { DashboardState } from '@/crystal_ball/types';

/**
 * Props for {@link DeleteDashboardDialog}.
 */
interface DeleteDashboardDialogProps {
  /** Whether the modal is open. */
  open: boolean;
  /** Callback for when the modal is closed. */
  onClose: () => void;
  /** Callback for when the user confirms the deletion. */
  onConfirm: () => void;
  /** Whether the deletion is in progress. */
  isDeleting: boolean;
  /** The dashboard to delete. */
  dashboardState: DashboardState | null;
}

/**
 * A dialog component for deleting a dashboard.
 */
export function DeleteDashboardDialog({
  open,
  onClose,
  onConfirm,
  isDeleting,
  dashboardState,
}: DeleteDashboardDialogProps) {
  const name =
    dashboardState?.displayName || dashboardState?.name || 'this dashboard';

  return (
    <Dialog open={open} onClose={isDeleting ? undefined : onClose}>
      <DialogTitle>Delete Dashboard</DialogTitle>
      <DialogContent>
        <DialogContentText>
          Are you sure you want to delete <strong>{name}</strong>? Deleted
          dashboards are kept in the system for 30 days.
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isDeleting} color="inherit">
          Cancel
        </Button>
        <Button onClick={onConfirm} disabled={isDeleting} color="error">
          {isDeleting ? 'Deleting...' : 'Delete'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
