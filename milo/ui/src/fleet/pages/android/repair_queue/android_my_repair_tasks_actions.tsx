// Copyright 2026 The LUCI Authors.
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

import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Box, Button, Typography } from '@mui/material';
import { useMemo } from 'react';

import {
  UNKNOWN_USER_EMAIL,
  useCurrentUserEmail,
} from '@/fleet/components/repair_queue/use_current_user_email';
import { radii } from '@/fleet/theme/tokens';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { ListAndroidRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { toAndroidWorkspaceEnum } from './android_workspace';
import {
  getClaimedByFilter,
  getDeviceListLinkForRepairTasks,
} from './my_repair_tasks';
import { useAndroidRepairQueue } from './use_android_repair_queue';

// Upper bound on the number of claimed tasks fetched for the current user.
// A technician only works on a handful of tasks at a time.
const MY_TASKS_PAGE_SIZE = 100;

export interface AndroidMyRepairTasksActionsProps {
  workspace: AndroidPageWorkspace;
}

/**
 * Button that opens the repair tasks claimed by the current user in the
 * Fleet Console device list. Hidden when the user has no claimed task.
 */
export const AndroidMyRepairTasksActions = ({
  workspace,
}: AndroidMyRepairTasksActionsProps) => {
  const currentUser = useCurrentUserEmail();
  const isKnownUser = currentUser !== UNKNOWN_USER_EMAIL;

  const request = useMemo(
    () =>
      ListAndroidRepairQueueRequest.fromPartial({
        pageSize: MY_TASKS_PAGE_SIZE,
        pageToken: '',
        orderBy: '',
        filter: getClaimedByFilter(currentUser),
        workspace: toAndroidWorkspaceEnum(workspace),
      }),
    [currentUser, workspace],
  );
  const queueQuery = useAndroidRepairQueue(request, { enabled: isKnownUser });

  const myTasks = queueQuery.data?.repairQueueItems ?? [];

  // Empty when the user has no claimed task.
  const deviceListLink = getDeviceListLinkForRepairTasks(workspace, myTasks);

  if (!deviceListLink) {
    return null;
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
      <Typography variant="body2" color="text.secondary">
        My tasks ({myTasks.length}):
      </Typography>
      <Button
        variant="outlined"
        size="small"
        href={deviceListLink}
        target="_blank"
        rel="noopener noreferrer"
        endIcon={<OpenInNewIcon />}
        sx={{ textTransform: 'none', borderRadius: radii.sm }}
      >
        Show devices
      </Button>
    </Box>
  );
};
