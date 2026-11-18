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

import { actionRowSx } from '@/fleet/theme/recipes';

import {
  buildRepairTasksDeviceListLink,
  getMyClaimedRepairTasks,
} from './my_repair_tasks';
import { useCurrentUserEmail } from './use_current_user_email';

const EMPTY_REPAIR_TASKS: readonly {
  readonly deviceId: string;
  readonly claimedBy?: string;
}[] = [];

export interface MyRepairTasksActionsProps {
  /**
   * First page of the repair queue, where the backend pins tasks claimed by
   * the authenticated user to the top.
   */
  readonly items?: readonly {
    readonly deviceId: string;
    readonly claimedBy?: string;
  }[];
  readonly platformBaseUrl: string;
  readonly idFilterKey?: string;
}

/**
 * Toolbar button that opens the repair tasks claimed by the current user in the
 * platform's Fleet Console device list. Hidden when the user has no claimed task.
 */
export const MyRepairTasksActions = ({
  items = EMPTY_REPAIR_TASKS,
  platformBaseUrl,
  idFilterKey = 'id',
}: MyRepairTasksActionsProps) => {
  const currentUser = useCurrentUserEmail();

  const { taskCount, deviceListLink } = useMemo(() => {
    const myTasks = getMyClaimedRepairTasks(items, currentUser);
    return {
      taskCount: myTasks.length,
      deviceListLink: buildRepairTasksDeviceListLink(
        myTasks,
        platformBaseUrl,
        idFilterKey,
      ),
    };
  }, [currentUser, idFilterKey, items, platformBaseUrl]);

  if (!deviceListLink) {
    return null;
  }

  return (
    <Box sx={actionRowSx}>
      <Typography variant="body2" color="text.secondary">
        My tasks ({taskCount}):
      </Typography>
      <Button
        variant="toolbar"
        href={deviceListLink}
        target="_blank"
        rel="noopener noreferrer"
        endIcon={<OpenInNewIcon />}
      >
        Show devices
      </Button>
    </Box>
  );
};
