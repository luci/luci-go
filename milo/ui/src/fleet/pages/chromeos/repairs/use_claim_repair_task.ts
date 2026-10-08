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

import { useAuthState } from '@/common/components/auth_state_provider';
import { useRepairQueueOptimisticMutation } from '@/fleet/components/repair_queue/use_repair_queue_optimistic_mutation';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ClaimRepairTaskRequest,
  ClaimRepairTaskResponse,
  RepairQueueItem,
  UnclaimRepairTaskRequest,
  UnclaimRepairTaskResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { REPAIR_QUEUE_QUERY_KEY } from './use_repair_queue';

export const useClaimRepairTask = () => {
  const client = useFleetConsoleClient();
  const authState = useAuthState();
  const email = authState.email?.trim();
  const rawIdentity = authState.identity?.trim();
  const identity = rawIdentity?.startsWith('user:')
    ? rawIdentity.replace(/^user:/, '')
    : rawIdentity;
  const currentUser = email || identity || 'current_user';

  return useRepairQueueOptimisticMutation<
    ClaimRepairTaskResponse,
    ClaimRepairTaskRequest,
    RepairQueueItem
  >({
    queryKey: REPAIR_QUEUE_QUERY_KEY,
    mutationFn: (req: ClaimRepairTaskRequest) => client.ClaimRepairTask(req),
    updateItem: (item) => ({
      ...item,
      claimedBy: currentUser,
      claimedAt: new Date().toISOString(),
    }),
  });
};

export const useUnclaimRepairTask = () => {
  const client = useFleetConsoleClient();

  return useRepairQueueOptimisticMutation<
    UnclaimRepairTaskResponse,
    UnclaimRepairTaskRequest,
    RepairQueueItem
  >({
    queryKey: REPAIR_QUEUE_QUERY_KEY,
    mutationFn: (req: UnclaimRepairTaskRequest) =>
      client.UnclaimRepairTask(req),
    updateItem: (item) => ({
      ...item,
      claimedBy: '',
      claimedAt: undefined,
    }),
  });
};
