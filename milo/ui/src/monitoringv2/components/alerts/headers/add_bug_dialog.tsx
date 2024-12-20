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

interface AddBugDialogProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (bugId: string) => void;
}

export const AddBugDialog: React.FC<AddBugDialogProps> = ({
  open,
  onClose,
  onSubmit,
}) => {
  const [bugId, setBugId] = useState('');
  const [error, setError] = useState('');

  const handleClose = () => {
    setBugId('');
    setError('');
    onClose();
  };

  const handleSubmit = () => {
    if (!bugId) {
      setError('Please enter a bug ID.');
      return;
    }

    if (isNaN(parseInt(bugId))) {
      setError('Invalid bug ID. Please enter a number.');
      return;
    }

    onSubmit(bugId);
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
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button onClick={handleSubmit} variant="contained">
          Add Bug
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddBugDialog;
