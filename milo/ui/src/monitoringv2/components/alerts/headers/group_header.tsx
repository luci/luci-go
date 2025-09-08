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

import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button, IconButton, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { GroupBugTable } from '../../bug_table';

import { AddBugDialog } from './add_bug_dialog';
import { DeleteGroupDialog } from './delete_group_dialog';
import { EditGroupNameDialog } from './edit_group_name_dialog';
import { EditGroupStatusMessageDialog } from './edit_group_status_message_dialog';

interface GroupHeaderProps {
  group: AlertGroup;
}

export const GroupHeader = ({ group }: GroupHeaderProps) => {
  const [showEditNameDialog, setShowEditNameDialog] = useState(false);
  const [showEditStatusMessageDialog, setShowEditStatusMessageDialog] =
    useState(false);
  const [showDeleteGroupDialog, setShowDeleteDialog] = useState(false);

  const [showAddBugDialog, setShowAddBugDialog] = useState(false);

  return (
    <Box sx={{ padding: '16px' }}>
      <Typography variant="h5">
        {group.displayName}
        <IconButton onClick={() => setShowEditNameDialog(true)}>
          <EditIcon />
        </IconButton>
        {showEditNameDialog ? (
          <EditGroupNameDialog
            group={group}
            onClose={() => setShowEditNameDialog(false)}
          />
        ) : null}
        <Button
          variant="outlined"
          color="inherit"
          startIcon={<DeleteIcon />}
          onClick={() => setShowDeleteDialog(true)}
          sx={{ opacity: '70%' }}
        >
          Delete
        </Button>
        {showDeleteGroupDialog ? (
          <DeleteGroupDialog
            group={group}
            onClose={() => setShowDeleteDialog(false)}
          />
        ) : null}
        <span style={{ fontSize: '14px', opacity: '70%', paddingLeft: '12px' }}>
          Updated by{' '}
          <a
            href={`http://who/${group.updatedBy}`}
            target="_blank"
            rel="noreferrer"
          >
            {group.updatedBy}
          </a>{' '}
          {group.updateTime ? (
            <RelativeTimestamp
              timestamp={DateTime.fromISO(group.updateTime)}
              formatFn={displayApproxDuration}
            ></RelativeTimestamp>
          ) : null}
          .
        </span>
      </Typography>

      <Typography variant="body1">
        {group.statusMessage || (
          <em style={{ opacity: '50%' }}>
            No status message has been entered yet.
          </em>
        )}
        <IconButton onClick={() => setShowEditStatusMessageDialog(true)}>
          <EditIcon />
        </IconButton>
        {showEditStatusMessageDialog ? (
          <EditGroupStatusMessageDialog
            group={group}
            onClose={() => setShowEditStatusMessageDialog(false)}
          />
        ) : null}
      </Typography>

      <Box>
        {group.bugs.length > 0 && <GroupBugTable group={group} />}
        <Button
          onClick={() => setShowAddBugDialog(true)}
          startIcon={<AddIcon />}
          color="inherit"
          variant="outlined"
          size="small"
          sx={{ margin: '12px 0' }}
        >
          Add Bug
        </Button>
        {showAddBugDialog ? (
          <AddBugDialog
            group={group}
            open={true}
            onClose={() => setShowAddBugDialog(false)}
          />
        ) : null}
      </Box>
    </Box>
  );
};
