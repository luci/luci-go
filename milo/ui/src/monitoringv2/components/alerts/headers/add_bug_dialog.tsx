// Copyright 2024 The LUCI Authors.
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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@mui/material';
import React, { useState } from 'react';

import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

interface AddBugDialogProps {
  group: AlertGroup;
  open: boolean;
  onClose: () => void;
}

export const AddBugDialog: React.FC<AddBugDialogProps> = ({
  open,
  onClose,
  group,
}) => {
  const [bugId, setBugId] = useState('');
  const [error, setError] = useState('');
  const { update: updateGroup } = useAlertGroups();
  const handleClose = () => {
    setBugId('');
    setError('');
    onClose();
  };

  const handleSubmit = async () => {
    if (!bugId) {
      setError('Please enter a bug ID.');
      return;
    }

    if (isNaN(parseInt(bugId))) {
      setError('Invalid bug ID. Please enter a number.');
      return;
    }

    await updateGroup.mutateAsync({
      alertGroup: {
        ...group,
        bugs: [...group.bugs.filter((b) => b !== bugId), bugId],
      },
      updateMask: ['bugs'],
    });
    handleClose();
  };

  return (
    <Dialog open={open} onClose={handleClose}>
      <DialogTitle>Add Bug</DialogTitle>
      <DialogContent>
        <Typography gutterBottom>
          Enter the bug number to link to this alert.
        </Typography>
        <TextField
          margin="dense"
          id="bug-id"
          label="Bug ID"
          type="text"
          fullWidth
          value={bugId}
          onChange={(e) => setBugId(e.target.value)}
          error={!!error}
          helperText={error}
          disabled={updateGroup.isPending}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={updateGroup.isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          loading={updateGroup.isPending}
        >
          Add Bug
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddBugDialog;
