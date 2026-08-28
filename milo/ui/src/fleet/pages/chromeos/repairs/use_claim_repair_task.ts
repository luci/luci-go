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

import { useMutation, useQueryClient } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ClaimRepairTaskRequest,
  ClaimRepairTaskResponse,
  ListRepairQueueResponse,
  RepairQueueItem,
  UnclaimRepairTaskRequest,
  UnclaimRepairTaskResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { REPAIR_QUEUE_QUERY_KEY } from './use_repair_queue';

interface UseRepairQueueOptimisticMutationOptions<
  TData,
  TVariables extends { taskId: string },
> {
  mutationFn: (variables: TVariables) => Promise<TData>;
  updateItem: (item: RepairQueueItem, variables: TVariables) => RepairQueueItem;
}

export const useRepairQueueOptimisticMutation = <
  TData,
  TVariables extends { taskId: string },
>({
  mutationFn,
  updateItem,
}: UseRepairQueueOptimisticMutationOptions<TData, TVariables>) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onMutate: async (variables: TVariables) => {
      await queryClient.cancelQueries({ queryKey: REPAIR_QUEUE_QUERY_KEY });

      const previousQueries =
        queryClient.getQueriesData<ListRepairQueueResponse>({
          queryKey: REPAIR_QUEUE_QUERY_KEY,
        });

      const targetTaskId = variables.taskId;

      queryClient.setQueriesData<ListRepairQueueResponse>(
        { queryKey: REPAIR_QUEUE_QUERY_KEY },
        (oldData) => {
          if (!oldData || !oldData.repairQueueItems) {
            return oldData;
          }
          return {
            ...oldData,
            repairQueueItems: oldData.repairQueueItems.map((item) => {
              if (item.taskId === targetTaskId) {
                return updateItem(item, variables);
              }
              return item;
            }),
          };
        },
      );

      return { previousQueries };
    },
    onError: (_err, _variables, context) => {
      if (context?.previousQueries) {
        for (const [queryKey, data] of context.previousQueries) {
          queryClient.setQueryData(queryKey, data);
        }
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: REPAIR_QUEUE_QUERY_KEY });
    },
  });
};

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
    ClaimRepairTaskRequest
  >({
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
    UnclaimRepairTaskRequest
  >({
    mutationFn: (req: UnclaimRepairTaskRequest) =>
      client.UnclaimRepairTask(req),
    updateItem: (item) => ({
      ...item,
      claimedBy: '',
      claimedAt: undefined,
    }),
  });
};
