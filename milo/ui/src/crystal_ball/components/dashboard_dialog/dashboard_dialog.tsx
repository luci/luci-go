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

import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import TextField from '@mui/material/TextField';
import { useEffect, useState } from 'react';

import { DashboardState } from '@/crystal_ball/types';

/**
 * Props for {@link DashboardDialog}.
 */
export interface DashboardDialogProps {
  /** Whether the modal is open. */
  open: boolean;
  /** Callback for when the modal is closed. */
  onClose: () => void;
  /** Let the parent pass initial data */
  initialData?: DashboardState;
  /** A single, unified submit handler */
  onSubmit: (data: {
    displayName: string;
    description: string;
  }) => Promise<void>;
  /** Let the parent tell the dialog if it's currently saving */
  isPending?: boolean;
  /** Let the parent handle/pass down the error */
  errorMsg?: string;
  /** Optional override for the button text */
  submitText?: string;
}

/**
 * A generic dialog component for creating or editing a dashboard.
 * All API and state logic is delegated to the parent handler 'onSubmit'.
 */
export function DashboardDialog({
  open,
  onClose,
  initialData,
  onSubmit,
  isPending = false,
  errorMsg = '',
  submitText = 'Save',
}: DashboardDialogProps) {
  const isEditing = !!initialData;
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (open) {
      setName(initialData?.displayName || '');
      setDescription(initialData?.description || '');
    } else {
      setName('');
      setDescription('');
    }
  }, [open, initialData]);

  const handleSubmit = async () => {
    await onSubmit({ displayName: name, description });
  };

  const isFormValid = name.trim().length > 0;

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>
        {isEditing ? 'Edit Dashboard Details' : 'Create New Dashboard'}
      </DialogTitle>
      <DialogContent>
        {errorMsg && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errorMsg}
          </Alert>
        )}
        <TextField
          margin="dense"
          label="Dashboard Name"
          type="text"
          fullWidth
          variant="outlined"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <TextField
          margin="dense"
          label="Description"
          type="text"
          fullWidth
          variant="outlined"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          multiline
          rows={3}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={!isFormValid || isPending}
        >
          {isPending ? 'Saving...' : submitText}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
