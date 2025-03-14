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
} from '@mui/material';
import { useState } from 'react';

import { AlertGroup } from '../alerts';

interface EditGroupStatusMessageDialogProps {
  group: AlertGroup;
  setGroup: (group: AlertGroup) => void;
  onClose: () => void;
}

/**
 * A dialog to edit the group status message.
 */
export const EditGroupStatusMessageDialog = ({
  group,
  setGroup,
  onClose,
}: EditGroupStatusMessageDialogProps) => {
  const [groupStatusMessage, setGroupStatusMessage] = useState(
    group.statusMessage,
  );

  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>Edit Status Message</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="name"
          label="Status message"
          type="text"
          fullWidth
          multiline
          rows={4}
          variant="outlined"
          value={groupStatusMessage}
          onChange={(e) => setGroupStatusMessage(e.target.value)}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          onClick={() => {
            setGroup({ ...group, statusMessage: groupStatusMessage });
            onClose();
          }}
        >
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
};
