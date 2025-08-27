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

import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

interface EditGroupStatusMessageDialogProps {
  group: AlertGroup;
  onClose: () => void;
}

/**
 * A dialog to edit the group status message.
 */
export const EditGroupStatusMessageDialog = ({
  group,
  onClose,
}: EditGroupStatusMessageDialogProps) => {
  const [groupStatusMessage, setGroupStatusMessage] = useState(
    group.statusMessage,
  );

  const { update: updateGroup } = useAlertGroups();

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="md">
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
          disabled={updateGroup.isPending}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={updateGroup.isPending}>
          Cancel
        </Button>
        <Button
          onClick={async () => {
            await updateGroup.mutateAsync({
              alertGroup: { ...group, statusMessage: groupStatusMessage },
              updateMask: ['status_message'],
            });
            onClose();
          }}
          loading={updateGroup.isPending}
        >
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
};
