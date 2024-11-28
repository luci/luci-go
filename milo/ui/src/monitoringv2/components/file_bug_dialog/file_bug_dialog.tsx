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
  Alert,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Link,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useNotifyAlertsClient } from '@/monitoringv2/hooks/prpc_clients';
import { TreeJson } from '@/monitoringv2/util/server_json';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { fileBugLink } from './file_bug_link';

interface FileBugDialogProps {
  alerts: string[];
  tree: TreeJson;
  open: boolean;
  onClose: () => void;
}

export const FileBugDialog = ({
  alerts,
  tree,
  open,
  onClose,
}: FileBugDialogProps) => {
  const [bugId, setBugId] = useState('');
  const queryClient = useQueryClient();
  const client = useNotifyAlertsClient();
  const linkBugMutation = useMutation({
    mutationFn: (bug: string) => {
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: alerts.map((a) => {
            return UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(a)}`,
                bug: bug,
                // FIXME!
                silenceUntil: '0', // a.silenceUntil,
              },
            });
          }),
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries();
      onClose();
    },
  });
  if (!tree) {
    return null;
  }
  if (linkBugMutation.isLoading) {
    <Dialog open={open} onClose={onClose}>
      <DialogTitle>Link bug</DialogTitle>
      <CircularProgress></CircularProgress>
    </Dialog>;
  }

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogTitle>Link bug</DialogTitle>
      <DialogContent>
        {linkBugMutation.isError ? (
          <Alert severity="error">
            Error linking bug: {(linkBugMutation.error as Error).message}
          </Alert>
        ) : null}
        <Typography>
          To link a bug to this alert, please enter the bug ID in the box below.
        </Typography>
        <TextField
          sx={{ marginTop: '10px' }}
          label="Bug ID"
          value={bugId}
          onChange={(e) => setBugId(e.target.value)}
        />
        <Typography>
          If you don&apos;t yet have a bug, please use{' '}
          <Link target="_blank" href={fileBugLink(tree, alerts)}>
            this link to create a bug
          </Link>{' '}
          for this alert.
        </Typography>
        <Typography>
          Once it is created, please copy the bug id into the box to link the
          bug.
        </Typography>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onClose()}>Close</Button>
        <Button onClick={() => linkBugMutation.mutate(bugId)}>Link Bug</Button>
      </DialogActions>
    </Dialog>
  );
};
