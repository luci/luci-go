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

import { FileBugDialog } from '@/monitoring/components/file_bug_dialog/file_bug_dialog';
import { linkBug, unlinkBug } from '@/monitoring/util/bug_annotations';
import { AlertJson, Bug, BugId, TreeJson } from '@/monitoring/util/server_json';

interface BugMenuProps {
  anchorEl: HTMLElement | null;
  onClose: () => void;
  tree: TreeJson;
  alerts: AlertJson[];
  bugs: Bug[];
  alertBugs: { [alertKey: string]: BugId[] };
}
// TODO(b/319315200): Dialog to confirm multiple alert bug linking
// TODO(b/319315200): Unlink before linking to another bug + dialog to confirm it
export const BugMenu = ({
  anchorEl,
  onClose,
  alerts,
  tree,
  bugs,
  alertBugs,
}: BugMenuProps) => {
  const [linkBugOpen, setLinkBugOpen] = useState(false);
  const open = Boolean(anchorEl) && !linkBugOpen;
  const queryClient = useQueryClient();

  const isLinkedToBugs =
    alerts.filter((a) => alertBugs[a.key]?.length > 0).length > 0;

  const linkBugMutation = useMutation({
    mutationFn: (bug: string) => {
      return linkBug(tree, alerts, bug);
    },
    onSuccess: () => queryClient.invalidateQueries(['annotations']),
  });
  const unlinkBugMutation = useMutation({
    mutationFn: () => {
      const promises: Promise<unknown>[] = [];
      for (const alert of alerts) {
        promises.push(unlinkBug(tree, alert, alertBugs[alert.key]));
      }
      return Promise.all(promises);
    },
    onSuccess: () => queryClient.invalidateQueries(['annotations']),
  });
  return (
    <>
      <Menu id="basic-menu" anchorEl={anchorEl} open={open} onClose={onClose}>
        {isLinkedToBugs ? (
          <>
            <MenuItem
              onClick={(e) => {
                e.stopPropagation();
                unlinkBugMutation.mutateAsync().finally(() => onClose());
              }}
            >
              Unlink bug {alerts.length !== 1 && 'from all alerts'}
            </MenuItem>
            <Divider />
          </>
        ) : null}
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
          Create bug...
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
      <Snackbar open={linkBugMutation.isError}>
        <Alert severity="error">
          Error saving bug links: {linkBugMutation.error as string}
        </Alert>
      </Snackbar>
    </>
  );
};
