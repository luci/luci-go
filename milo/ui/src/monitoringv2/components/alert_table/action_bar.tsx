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

import CreateNewFolderIcon from '@mui/icons-material/CreateNewFolder';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import FolderDeleteIcon from '@mui/icons-material/FolderDelete';
import NotificationsPausedIcon from '@mui/icons-material/NotificationsPaused';
import {
  Box,
  IconButton,
  Link,
  TableCell,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useNotifyAlertsClient } from '@/monitoring/hooks/prpc_clients';
import { StructuredAlert } from '@/monitoringv2/util/alerts';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { AlertGroup } from '../alerts';

import { SelectGroupMenu } from './bug_menu';
import { CreateGroupDialog } from './create_group_dialog';
import { RemoveFromGroupDialog } from './remove_from_group_dialog';

interface ActionBarProps {
  group: AlertGroup | undefined;
  groups: AlertGroup[];
  setGroups: (groups: AlertGroup[]) => void;
  alerts: StructuredAlert[];
  selectedAlertKeys: { [key: string]: boolean };
  unselectAll: () => void;
}

export const ActionBar = ({
  group,
  groups,
  setGroups,
  alerts,
  selectedAlertKeys,
  unselectAll,
}: ActionBarProps) => {
  const [showCreateGroupDialog, setShowCreateGroupDialog] = useState(false);
  const [showRemoveFromGroupDialog, setShowRemoveFromGroupDialog] =
    useState(false);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const authState = useAuthState();

  const queryClient = useQueryClient();
  const client = useNotifyAlertsClient();
  const silenceAllMutation = useMutation({
    mutationFn: (alerts: StructuredAlert[]) => {
      /* FIXME! do step and test alerts too. */
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: alerts.map((a) =>
            UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(a.alert.key)}`,
                // FIXME!
                bug: '0', // a.bug || '0',
                silenceUntil: `${a.alert.history[0].buildId || 0}`,
              },
            }),
          ),
        }),
      );
    },
    onSuccess: () => queryClient.invalidateQueries(),
  });

  const createGroupAndUnselect = (group: AlertGroup) => {
    group.id = new Date().toISOString();
    group.updated = group.id;
    group.updatedBy = authState.email?.split('@')[0];

    setGroups([...groups, group]);
    unselectAll();
  };

  const removeFromGroupAndUnselect = (
    group: AlertGroup,
    alertKeys: string[],
  ) => {
    setGroups(
      groups.map((g) => {
        if (g.id === group.id) {
          return {
            ...g,
            alertKeys: g.alertKeys.filter((k) => !alertKeys.includes(k)),
          };
        } else {
          return g;
        }
      }),
    );
    unselectAll();
  };
  const moveToGroupAndUnselect = (group: AlertGroup) => {
    const groupsCopy = groups.map((g) => {
      const groupCopy = {
        ...g,
        alertKeys: g.alertKeys.filter((k) => !selectedAlertKeys[k]),
      };
      if (g.id === group.id) {
        groupCopy.alertKeys.push(...Object.keys(selectedAlertKeys));
      }
      return groupCopy;
    });
    setGroups(groupsCopy);
    unselectAll();
  };

  return (
    <>
      <TableCell colSpan={4}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <Tooltip title="Create group">
            <IconButton onClick={() => setShowCreateGroupDialog(true)}>
              <CreateNewFolderIcon />
            </IconButton>
          </Tooltip>
          {showCreateGroupDialog ? (
            <CreateGroupDialog
              onClose={() => setShowCreateGroupDialog(false)}
              alerts={alerts.filter((a) => selectedAlertKeys[a.alert.key])}
              createGroup={createGroupAndUnselect}
            />
          ) : null}

          <Tooltip title="Move to existing group">
            <IconButton onClick={(e) => setMenuAnchorEl(e.currentTarget)}>
              <DriveFileMoveIcon />
            </IconButton>
          </Tooltip>
          <SelectGroupMenu
            anchorEl={menuAnchorEl}
            onSelect={(group) => moveToGroupAndUnselect(group)}
            onClose={() => setMenuAnchorEl(null)}
            groups={groups}
          />
          {group !== undefined ? (
            <>
              <Tooltip title="Remove from group">
                <IconButton onClick={() => setShowRemoveFromGroupDialog(true)}>
                  <FolderDeleteIcon />
                </IconButton>
              </Tooltip>
              {showRemoveFromGroupDialog ? (
                <RemoveFromGroupDialog
                  onClose={() => setShowRemoveFromGroupDialog(false)}
                  alerts={alerts.filter((a) => selectedAlertKeys[a.alert.key])}
                  group={group}
                  onConfirm={() =>
                    removeFromGroupAndUnselect(
                      group,
                      alerts.map((a) => a.alert.key),
                    )
                  }
                />
              ) : null}
            </>
          ) : null}
          <Tooltip title="Snooze until next build">
            <IconButton onClick={() => silenceAllMutation.mutate(alerts)}>
              <NotificationsPausedIcon />
            </IconButton>
          </Tooltip>
          <div style={{ flexGrow: 1 }}></div>
          <Typography style={{ paddingRight: '24px', fontWeight: 'bold' }}>
            <Link
              href={`/b/FIXME/blamelist`}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
            >
              Combined blamelist: XX CLs
            </Link>
          </Typography>
        </Box>
      </TableCell>
    </>
  );
};
