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
import { useState } from 'react';
import { useNavigate } from 'react-router';

import { useCreateDashboardState } from '@/crystal_ball/hooks/use_dashboard_state_api';
import { extractIdFromName, formatApiError } from '@/crystal_ball/utils';

/**
 * Props for {@link CreateDashboardModal}.
 */
export interface CreateDashboardModalProps {
  /** Whether the modal is open. */
  open: boolean;
  /** Callback for when the modal is closed. */
  onClose: () => void;
}

/**
 * A modal component for creating a new dashboard.
 */
export function CreateDashboardModal({
  open,
  onClose,
}: CreateDashboardModalProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const createMutation = useCreateDashboardState();
  const navigate = useNavigate();
  const [errorMsg, setErrorMsg] = useState('');

  const handleCreate = async () => {
    setErrorMsg('');
    try {
      const response = await createMutation.mutateAsync({
        dashboardState: {
          displayName: name,
          description: description,
          dashboardContent: {},
        },
      });

      const parsedResp = response.response;

      const newName = parsedResp?.name;
      if (newName) {
        const newId = extractIdFromName(newName);
        navigate(`/ui/labs/crystal-ball/dashboards/${newId}`);
      }
      onClose();
    } catch (e) {
      setErrorMsg(formatApiError(e, 'Failed to create dashboard'));
    }
  };

  const isFormValid = name.trim().length > 0;

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Create New Dashboard</DialogTitle>
      <DialogContent>
        {errorMsg && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errorMsg}
          </Alert>
        )}
        <TextField
          margin="dense"
          id="name"
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
          id="description"
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
        <Button onClick={onClose}>Cancel</Button>
        <Button
          onClick={handleCreate}
          variant="contained"
          disabled={!isFormValid || createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating...' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
