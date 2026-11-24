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

import { QueryKey, useMutation, useQueryClient } from '@tanstack/react-query';

/** The fields of a repair queue item that the optimistic update reads. */
export interface OptimisticRepairQueueItem {
  readonly taskId: string;
  readonly claimedBy?: string;
  readonly claimedAt?: string;
  readonly etag?: string;
}

/** Mutation response shape carrying the updated repair queue item and fresh ETag. */
export interface OptimisticRepairQueueMutationResponse<
  TItem extends OptimisticRepairQueueItem,
> {
  readonly repairQueueItem?: TItem;
}

/** The fields of a list repair queue response that the optimistic update reads. */
export interface OptimisticRepairQueueResponse<
  TItem extends OptimisticRepairQueueItem,
> {
  readonly repairQueueItems: readonly TItem[];
  readonly inProgressCount?: number;
}

interface UseRepairQueueOptimisticMutationOptions<
  TData extends OptimisticRepairQueueMutationResponse<TItem> | void,
  TVariables extends { taskId: string },
  TItem extends OptimisticRepairQueueItem,
> {
  queryKey: QueryKey;
  invalidateQueryKeys?: readonly QueryKey[];
  mutationFn: (variables: TVariables) => Promise<TData>;
  updateItem: (item: TItem, variables: TVariables) => TItem;
  onError?: (err: Error, variables: TVariables) => void;
}

export const useRepairQueueOptimisticMutation = <
  TData extends OptimisticRepairQueueMutationResponse<TItem> | void,
  TVariables extends { taskId: string },
  TItem extends OptimisticRepairQueueItem,
>({
  queryKey,
  invalidateQueryKeys,
  mutationFn,
  updateItem,
  onError,
}: UseRepairQueueOptimisticMutationOptions<TData, TVariables, TItem>) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onMutate: async (variables: TVariables) => {
      await queryClient.cancelQueries({ queryKey });

      const previousQueries = queryClient.getQueriesData<
        OptimisticRepairQueueResponse<TItem>
      >({
        queryKey,
      });

      const targetTaskId = variables.taskId;

      let inProgressDelta = 0;
      for (const [, data] of previousQueries) {
        const targetItem = data?.repairQueueItems?.find(
          (item) => item.taskId === targetTaskId,
        );
        if (targetItem) {
          const wasClaimed = Boolean(targetItem.claimedBy?.trim());
          const updated = updateItem(targetItem, variables);
          const isClaimed = Boolean(updated.claimedBy?.trim());
          if (!wasClaimed && isClaimed) {
            inProgressDelta = 1;
          } else if (wasClaimed && !isClaimed) {
            inProgressDelta = -1;
          }
          break;
        }
      }

      queryClient.setQueriesData<OptimisticRepairQueueResponse<TItem>>(
        { queryKey },
        (oldData) => {
          if (!oldData || !oldData.repairQueueItems) {
            return oldData;
          }
          const updatedItems = oldData.repairQueueItems.map((item) =>
            item.taskId === targetTaskId ? updateItem(item, variables) : item,
          );
          return {
            ...oldData,
            repairQueueItems: updatedItems,
            inProgressCount: Math.max(
              0,
              (oldData.inProgressCount ?? 0) + inProgressDelta,
            ),
          };
        },
      );

      return { previousQueries };
    },
    onSuccess: (data, variables) => {
      const returnedItem = data ? data.repairQueueItem : undefined;
      if (!returnedItem) {
        return;
      }
      const targetTaskId = returnedItem.taskId || variables.taskId;
      queryClient.setQueriesData<OptimisticRepairQueueResponse<TItem>>(
        { queryKey },
        (oldData) => {
          if (!oldData || !oldData.repairQueueItems) {
            return oldData;
          }
          let matched = false;
          const updatedItems = oldData.repairQueueItems.map((item) => {
            if (item.taskId !== targetTaskId) {
              return item;
            }
            matched = true;
            const nextClaimedBy =
              returnedItem.claimedBy !== undefined
                ? returnedItem.claimedBy
                : item.claimedBy;
            const isClaimed = Boolean(nextClaimedBy?.trim());
            return {
              ...item,
              claimedBy: nextClaimedBy,
              claimedAt: isClaimed
                ? (returnedItem.claimedAt ?? item.claimedAt)
                : undefined,
              ...(returnedItem.etag !== undefined
                ? { etag: returnedItem.etag }
                : {}),
            };
          });
          return matched
            ? {
                ...oldData,
                repairQueueItems: updatedItems,
              }
            : oldData;
        },
      );
    },
    onError: (err, variables, context) => {
      if (context?.previousQueries) {
        for (const [key, data] of context.previousQueries) {
          queryClient.setQueryData(key, data);
        }
      }
      onError?.(err, variables);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey });
      if (invalidateQueryKeys) {
        for (const extraKey of invalidateQueryKeys) {
          queryClient.invalidateQueries({ queryKey: extraKey });
        }
      }
    },
  });
};
