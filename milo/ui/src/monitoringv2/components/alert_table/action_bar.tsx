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
import { Box, IconButton, TableCell, Tooltip } from '@mui/material';
import { useState } from 'react';

import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';
import { StructuredAlert } from '@/monitoringv2/util/alerts';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { SelectGroupMenu } from './bug_menu';
import { CreateGroupDialog } from './create_group_dialog';
import { RemoveFromGroupDialog } from './remove_from_group_dialog';

interface ActionBarProps {
  group: AlertGroup | undefined;
  groups: readonly AlertGroup[];
  alerts: StructuredAlert[];
  selectedAlertKeys: { [key: string]: boolean };
  unselectAll: () => void;
}

export const ActionBar = ({
  group,
  groups,
  alerts,
  selectedAlertKeys,
  unselectAll,
}: ActionBarProps) => {
  const [showCreateGroupDialog, setShowCreateGroupDialog] = useState(false);
  const [showRemoveFromGroupDialog, setShowRemoveFromGroupDialog] =
    useState(false);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);

  const { update: updateGroup } = useAlertGroups();

  const onCreate = () => {
    unselectAll();
  };

  const removeFromGroupAndUnselect = async (
    group: AlertGroup,
    alertKeys: string[],
  ) => {
    const alertNames = alertKeys.map((k) => `alerts/${encodeURIComponent(k)}`);
    await updateGroup.mutateAsync({
      alertGroup: {
        ...group,
        alertKeys: group.alertKeys.filter((k) => !alertNames.includes(k)),
      },
      updateMask: ['alert_keys'],
    });
    unselectAll();
  };
  const moveToGroupAndUnselect = async (group: AlertGroup) => {
    await updateGroup.mutateAsync({
      alertGroup: {
        ...group,
        alertKeys: [
          ...group.alertKeys,
          ...Object.keys(selectedAlertKeys).map(
            (k) => `alerts/${encodeURIComponent(k)}`,
          ),
        ],
      },
      updateMask: ['alert_keys'],
    });
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
              onCreate={onCreate}
              onClose={() => setShowCreateGroupDialog(false)}
              alertKeys={Object.keys(selectedAlertKeys)}
            />
          ) : null}

          <Tooltip title="Move to existing group">
            <IconButton onClick={(e) => setMenuAnchorEl(e.currentTarget)}>
              <DriveFileMoveIcon />
            </IconButton>
          </Tooltip>
          <SelectGroupMenu
            anchorEl={menuAnchorEl}
            onSelect={moveToGroupAndUnselect}
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
                      alerts
                        .filter((a) => selectedAlertKeys[a.alert.key])
                        .map((a) => a.alert.key),
                    )
                  }
                />
              ) : null}
            </>
          ) : null}
        </Box>
      </TableCell>
    </>
  );
};
