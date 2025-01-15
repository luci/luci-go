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
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useState } from 'react';

import { getLongestCommonSubstring } from '@/generic_libs/tools/string_utils';

import { AlertGroup } from '../alerts';

interface CreateGroupDialogProps {
  onClose: () => void;
  alertKeys: string[];
  createGroup: (group: AlertGroup) => void;
}

export const CreateGroupDialog = ({
  onClose,
  alertKeys: alertKeys,
  createGroup,
}: CreateGroupDialogProps) => {
  const defaultTitleParts = getLongestCommonSubstring(alertKeys)
    .split('/')
    .filter((s) => s);
  const defaultTitle = defaultTitleParts[defaultTitleParts.length - 1]
    .replace(/^[/.\s]+/, '')
    .replace(/[/.\s]+$/, '');
  const [name, setName] = useState(defaultTitle.length > 3 ? defaultTitle : '');
  const [statusMessage, setStatusMessage] = useState('Not yet investigated.');

  const group: AlertGroup = {
    id: new Date().toISOString(), // TODO(mwarton): replace with server generated id.
    name,
    statusMessage,
    bugs: [],
    alertKeys: alertKeys,
  };
  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>
        Create group from {alertKeys.length} alert
        {alertKeys.length > 1 ? 's' : ''}
      </DialogTitle>
      <DialogContent>
        <Box>
          <TextField
            label="Group name"
            sx={{ margin: '8px', width: '400px' }}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Box>
        <Box>
          <TextField
            multiline
            rows={4}
            label="Status message"
            sx={{ margin: '8px', width: '400px' }}
            value={statusMessage}
            onChange={(e) => setStatusMessage(e.target.value)}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onClose()}>Cancel</Button>
        <Button
          onClick={() => {
            createGroup(group);
            onClose();
          }}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
};
