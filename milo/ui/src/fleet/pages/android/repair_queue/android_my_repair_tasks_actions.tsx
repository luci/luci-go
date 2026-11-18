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

import { useMemo } from 'react';

import { MyRepairTasksActions } from '@/fleet/components/repair_queue/my_repair_tasks_actions';
import {
  UNKNOWN_USER_EMAIL,
  useCurrentUserEmail,
} from '@/fleet/components/repair_queue/use_current_user_email';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { ListAndroidRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { toAndroidWorkspaceEnum } from './android_workspace';
import { useAndroidRepairQueue } from './use_android_repair_queue';

// Upper bound on the number of claimed tasks fetched for the current user.
// A technician only works on a handful of tasks at a time, and the backend
// pins the authenticated caller's claimed tasks to the top of page 1.
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
        workspace: toAndroidWorkspaceEnum(workspace),
      }),
    [workspace],
  );
  const queueQuery = useAndroidRepairQueue(request, { enabled: isKnownUser });

  return (
    <MyRepairTasksActions
      items={queueQuery.data?.repairQueueItems}
      platformBaseUrl={workspaces[workspace].baseUrl}
      idFilterKey='"id"'
    />
  );
};
