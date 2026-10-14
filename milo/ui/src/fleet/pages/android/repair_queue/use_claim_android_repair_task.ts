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

import { useCurrentUserEmail } from '@/fleet/components/repair_queue/use_current_user_email';
import { useRepairQueueOptimisticMutation } from '@/fleet/components/repair_queue/use_repair_queue_optimistic_mutation';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  AndroidRepairQueueItem,
  ClaimAndroidRepairTaskRequest,
  ClaimAndroidRepairTaskResponse,
  UnclaimAndroidRepairTaskRequest,
  UnclaimAndroidRepairTaskResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ANDROID_REPAIR_QUEUE_QUERY_KEY } from './use_android_repair_queue';

export const useClaimAndroidRepairTask = () => {
  const client = useFleetConsoleClient();
  const currentUser = useCurrentUserEmail();

  return useRepairQueueOptimisticMutation<
    ClaimAndroidRepairTaskResponse,
    ClaimAndroidRepairTaskRequest,
    AndroidRepairQueueItem
  >({
    queryKey: ANDROID_REPAIR_QUEUE_QUERY_KEY,
    mutationFn: (req: ClaimAndroidRepairTaskRequest) =>
      client.ClaimAndroidRepairTask(req),
    updateItem: (item) => ({
      ...item,
      claimedBy: currentUser,
      claimedAt: new Date().toISOString(),
    }),
  });
};

export const useUnclaimAndroidRepairTask = () => {
  const client = useFleetConsoleClient();

  return useRepairQueueOptimisticMutation<
    UnclaimAndroidRepairTaskResponse,
    UnclaimAndroidRepairTaskRequest,
    AndroidRepairQueueItem
  >({
    queryKey: ANDROID_REPAIR_QUEUE_QUERY_KEY,
    mutationFn: (req: UnclaimAndroidRepairTaskRequest) =>
      client.UnclaimAndroidRepairTask(req),
    updateItem: (item) => ({
      ...item,
      claimedBy: '',
      claimedAt: undefined,
    }),
  });
};
