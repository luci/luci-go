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

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { FileBugDialog } from '@/monitoring/components/file_bug_dialog/file_bug_dialog';
import { AlertJson, Bug, TreeJson } from '@/monitoring/util/server_json';
import {
  AlertsClientImpl,
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

interface BugMenuProps {
  anchorEl: HTMLElement | null;
  onClose: () => void;
  tree: TreeJson;
  alerts: AlertJson[];
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

  const isLinkedToBugs = alerts.filter((a) => !!a.bug).length > 0;

  const client = usePrpcServiceClient({
    host: SETTINGS.luciNotify.host,
    ClientImpl: AlertsClientImpl,
  });
  const linkBugMutation = useMutation({
    mutationFn: (bug: string) => {
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: alerts.map((a) => {
            return UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(a.key)}`,
                bug: bug,
                silenceUntil: a.silenceUntil,
              },
            });
          }),
        }),
      );
    },
    onSuccess: () => queryClient.invalidateQueries(),
  });
  return (
    <>
      <Menu id="basic-menu" anchorEl={anchorEl} open={open} onClose={onClose}>
        {isLinkedToBugs ? (
          <MenuItem
            onClick={(e) => {
              e.stopPropagation();
              linkBugMutation.mutateAsync('0').finally(() => onClose());
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
              linkBugMutation
                .mutateAsync(`${bug.number}`)
                .finally(() => onClose());
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
