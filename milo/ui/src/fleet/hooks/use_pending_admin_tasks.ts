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

import { useAuthState } from '@/common/components/auth_state_provider';
import { useTasks } from '@/fleet/hooks/swarming_hooks';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { StateQuery } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useTasksClient } from '@/swarming/hooks/prpc_clients';

export const USER_TAG = 'client_user';
export const DEFAULT_PENDING_ADMIN_TASKS_LIMIT = 100;

export const useAdminTaskTags = (): string[] => {
  const authState = useAuthState();
  return useMemo(
    () => (authState.email ? [`${USER_TAG}:${authState.email}`] : []),
    [authState.email],
  );
};

export const usePendingAdminTasks = (
  {
    swarmingHost = DEVICE_TASKS_SWARMING_HOST,
    limit = DEFAULT_PENDING_ADMIN_TASKS_LIMIT,
    pageToken,
  }: {
    swarmingHost?: string;
    limit?: number;
    pageToken?: string;
  } = {},
  options?: { enabled?: boolean },
) => {
  const client = useTasksClient(swarmingHost);
  const adminTaskTags = useAdminTaskTags();

  return useTasks(
    {
      client,
      tags: adminTaskTags,
      limit,
      pageToken,
      state: StateQuery.QUERY_PENDING_RUNNING,
    },
    { enabled: (options?.enabled ?? true) && adminTaskTags.length > 0 },
  );
};

export const usePendingAdminTasksCount = ({
  swarmingHost = DEVICE_TASKS_SWARMING_HOST,
  enabled = true,
}: {
  swarmingHost?: string;
  enabled?: boolean;
} = {}): number => {
  const authState = useAuthState();
  const { tasks } = usePendingAdminTasks(
    { swarmingHost, limit: DEFAULT_PENDING_ADMIN_TASKS_LIMIT },
    { enabled: enabled && Boolean(authState.email) },
  );
  return authState.email ? tasks?.length || 0 : 0;
};
