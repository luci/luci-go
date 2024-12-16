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
  Typography,
} from '@mui/material';

import { AlertGroup } from '../alerts';

interface ArchiveGroupDialogProps {
  group: AlertGroup;
  archiveGroup: (group: AlertGroup) => void;
  onClose: () => void;
}
export const ArchiveGroupDialog = ({
  group,
  archiveGroup,
  onClose,
}: ArchiveGroupDialogProps) => {
  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>Archive Group</DialogTitle>
      <DialogContent>
        <Typography>
          Are you sure you want to archive the group{' '}
          <strong>{group.name}</strong>?
        </Typography>
        <Typography
          variant="body2"
          style={{ opacity: '80%', marginTop: '16px' }}
        >
          Archiving a group removes the group from the list of active groups on
          the sidebar. As alerts are no longer associated with an active group,
          they will be shown in the untriaged views. Groups will remain
          accessible by direct link for up to 14 days.
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          onClick={() => {
            archiveGroup({ ...group, alertKeys: [] });
            onClose();
          }}
        >
          Archive Group
        </Button>
      </DialogActions>
    </Dialog>
  );
};
