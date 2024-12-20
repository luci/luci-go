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
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button, Chip, IconButton, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { BugCard } from '@/common/components/bug_card';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuartion } from '@/common/tools/time_utils';

import { AlertGroup } from '../alerts';

import { AddBugDialog } from './add_bug_dialog';
import { ArchiveGroupDialog } from './archive_group_dialog';
import { EditGroupNameDialog } from './edit_group_name_dialog';
import { EditGroupStatusMessageDialog } from './edit_group_status_message_dialog';

interface GroupHeaderProps {
  group: AlertGroup;
  setGroup: (group: AlertGroup) => void;
  archiveGroup: (group: AlertGroup) => void;
}

export const GroupHeader = ({
  group,
  setGroup,
  archiveGroup,
}: GroupHeaderProps) => {
  const [showEditNameDialog, setShowEditNameDialog] = useState(false);
  const [showEditStatusMessageDialog, setShowEditStatusMessageDialog] =
    useState(false);
  const [showArchiveGroupDialog, setShowArchiveDialog] = useState(false);

  const [showAddBugDialog, setShowAddBugDialog] = useState(false);
  const [bugs, setBugs] = useState<string[]>(group.bugs || []);
  return (
    <Box sx={{ padding: '16px' }}>
      <Typography variant="h5">
        {group.name}
        <IconButton onClick={() => setShowEditNameDialog(true)}>
          <EditIcon />
        </IconButton>
        {showEditNameDialog ? (
          <EditGroupNameDialog
            group={group}
            setGroup={setGroup}
            onClose={() => setShowEditNameDialog(false)}
          />
        ) : null}
        <Button
          variant="outlined"
          color="inherit"
          startIcon={<DeleteIcon />}
          onClick={() => setShowArchiveDialog(true)}
          sx={{ opacity: '70%' }}
        >
          Archive
        </Button>
        {showArchiveGroupDialog ? (
          <ArchiveGroupDialog
            group={group}
            archiveGroup={archiveGroup}
            onClose={() => setShowArchiveDialog(false)}
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
          {group.updated ? (
            <RelativeTimestamp
              timestamp={DateTime.fromISO(group.updated)}
              formatFn={displayApproxDuartion}
            ></RelativeTimestamp>
          ) : null}
          .
        </span>
      </Typography>

      <Box>
        {bugs.map((bug) => (
          <HtmlTooltip key={bug} title={<BugCard bugId={bug} />}>
            <Chip
              onDelete={(e) => {
                setBugs(bugs.filter((b) => b !== bug));
                e.preventDefault();
                e.stopPropagation();
              }}
              label={`b/${bug}`}
              sx={{
                marginRight: '8px',
                marginBottom: '8px',
                cursor: 'pointer',
              }}
              component="a"
              href={`https://issues.chromium.org/issues/${bug}`}
              target="_blank"
              rel="noreferrer"
              deleteIcon={<CloseIcon />}
            />
          </HtmlTooltip>
        ))}
        <Button
          onClick={() => setShowAddBugDialog(true)}
          startIcon={<AddIcon />}
          color="inherit"
          sx={{ marginRight: '8px', marginBottom: '8px' }}
        >
          Add Bug
        </Button>
        {showAddBugDialog ? (
          <AddBugDialog
            open={true}
            onSubmit={(bugId) =>
              setBugs([...bugs.filter((b) => b !== bugId), bugId])
            }
            onClose={() => setShowAddBugDialog(false)}
          />
        ) : null}
      </Box>
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
            setGroup={setGroup}
            onClose={() => setShowEditStatusMessageDialog(false)}
          />
        ) : null}
      </Typography>
    </Box>
  );
};
