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
import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';

interface CreateGroupDialogProps {
  onCreate: () => void;
  onClose: () => void;
  alertKeys: string[];
}

export const CreateGroupDialog = ({
  onCreate,
  onClose,
  alertKeys: alertKeys,
}: CreateGroupDialogProps) => {
  const defaultTitleParts = getLongestCommonSubstring(alertKeys)
    .split('/')
    .filter((s) => s);
  const defaultTitle = defaultTitleParts[defaultTitleParts.length - 1]
    .replace(/^[/.\s]+/, '')
    .replace(/[/.\s]+$/, '');
  const [name, setName] = useState(defaultTitle.length > 3 ? defaultTitle : '');
  const [statusMessage, setStatusMessage] = useState('Not yet investigated.');
  const { create: createGroup } = useAlertGroups();

  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>
        Create group from {alertKeys.length} alert
        {alertKeys.length > 1 ? 's' : ''}
      </DialogTitle>
      <DialogContent>
        <Box
          sx={{
            mt: '8px',
            display: 'flex',
            flexDirection: 'column',
            gap: '16px',
          }}
        >
          <TextField
            label="Group name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={createGroup.isPending}
          />
          <TextField
            multiline
            rows={4}
            label="Status message"
            fullWidth
            value={statusMessage}
            onChange={(e) => setStatusMessage(e.target.value)}
            disabled={createGroup.isPending}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onClose()} disabled={createGroup.isPending}>
          Cancel
        </Button>
        <Button
          onClick={async () => {
            await createGroup.mutateAsync({
              displayName: name,
              statusMessage,
              alertKeys: alertKeys.map(
                (k) => `alerts/${encodeURIComponent(k)}`,
              ),
              bugs: [],
            });
            onCreate();
          }}
          loading={createGroup.isPending}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
};
