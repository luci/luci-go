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

import { Alert, Divider, Menu, MenuItem, Snackbar } from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { FileBugDialog } from '@/monitoringv2/components/file_bug_dialog/file_bug_dialog';
import { useNotifyAlertsClient } from '@/monitoringv2/hooks/prpc_clients';
import { Bug, TreeJson } from '@/monitoringv2/util/server_json';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

interface BugMenuProps {
  anchorEl: HTMLElement | null;
  onClose: () => void;
  tree: TreeJson;
  alerts: string[];
  bugs: Bug[];
}
// TODO(b/319315200): Dialog to confirm multiple alert bug linking
// TODO(b/319315200): Unlink before linking to another bug + dialog to confirm it
export const BugMenu = ({
  anchorEl,
  onClose,
  alerts,
  tree,
  bugs,
}: BugMenuProps) => {
  const [linkBugOpen, setLinkBugOpen] = useState(false);
  const open = Boolean(anchorEl) && !linkBugOpen;
  const queryClient = useQueryClient();

  // FIXME!
  const isLinkedToBugs = false; // alerts.filter((a) => !!a.bug).length > 0;

  const client = useNotifyAlertsClient();
  const linkBugMutation = useMutation({
    mutationFn: (bug: string) => {
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
    onSuccess: () => queryClient.invalidateQueries(),
    onSettled: () => onClose(),
  });
  return (
    <>
      <Menu id="basic-menu" anchorEl={anchorEl} open={open} onClose={onClose}>
        {isLinkedToBugs ? (
          <MenuItem
            onClick={(e) => {
              e.stopPropagation();
              linkBugMutation.mutateAsync('0');
            }}
          >
            Unlink bug {alerts.length !== 1 && 'from all alerts'}
          </MenuItem>
        ) : null}
        {
          // Do not merge this with the statement above otherwise <Menu /> will
          // complain that it does not support taking React fragment as a child.
          isLinkedToBugs ? <Divider /> : null
        }
        {bugs.map((bug) => (
          <MenuItem
            key={bug.link}
            onClick={(e) => {
              e.stopPropagation();
              linkBugMutation.mutateAsync(`${bug.number}`);
            }}
          >
            {bug.summary}
          </MenuItem>
        ))}
        {bugs.length > 0 ? <Divider /> : null}
        <MenuItem
          onClick={(e) => {
            e.stopPropagation();
            setLinkBugOpen(true);
          }}
        >
          Other/Create...
        </MenuItem>
      </Menu>
      <FileBugDialog
        alerts={alerts}
        tree={tree}
        open={linkBugOpen}
        onClose={() => {
          setLinkBugOpen(false);
          onClose();
        }}
      />
      <Snackbar open={linkBugMutation.isError} autoHideDuration={15000}>
        <Alert severity="error">
          Error saving bug links: {(linkBugMutation.error as Error)?.message}
        </Alert>
      </Snackbar>
    </>
  );
};
