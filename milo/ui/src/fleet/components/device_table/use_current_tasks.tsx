// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { useQueries, useQueryClient } from '@tanstack/react-query';
import _ from 'lodash';
import { useMemo } from 'react';

import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { extractDutId } from '@/fleet/utils/devices';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import {
  BotInfo,
  BotsRequest,
  NullableBool,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

export interface CurrentTasksResult {
  map: Map<string, string>;
  error: Error | null;
  isError: boolean;
  isPending: boolean;
}

/**
 * Fetches the current Swarming task for each DUT in the given list.
 * Batches requests to avoid server-side query limits.
 * @param devices An array of devices.
 * @param swarmingHost Swarming Host URL.
 * @param chunkSize Number of DUT ids to process in one request. Limited to 30, which is the
 * maximum number of values supported for a IN operation in datastore.
 * @returns An object containing the mapping of DUT ID to Task ID, pending state, and error state.
 */
export const useCurrentTasks = (
  devices: readonly Device[],
  options?: {
    chunkSize?: number;
    swarmingHost?: string;
  },
): CurrentTasksResult => {
  const { chunkSize = 25, swarmingHost = DEVICE_TASKS_SWARMING_HOST } =
    options ?? {};

  const swarmingClient = useBotsClient(swarmingHost);
  const dutIds = useMemo(() => devices.map(extractDutId), [devices]);

  // 1. Chunk the input dutIds into smaller arrays.
  const dutIdChunks = useMemo(
    // Sort the IDs first to ensure chunks are stable for the same set of IDs.
    () => _.chunk(_.sortBy(dutIds), chunkSize),
    [dutIds, chunkSize],
  );

  // 2. Use `useQueries` to create a separate query for each chunk.
  const queryClient = useQueryClient();
  const queries = useQueries({
    queries: dutIdChunks.map((chunk) => {
      const dutIdsString = chunk.join('|');
      return {
        queryKey: ['swarming-bots-current-tasks', dutIdsString],
        queryFn: async (): Promise<readonly BotInfo[]> => {
          const request = BotsRequest.fromPartial({
            dimensions: [{ key: 'dut_id', value: dutIdsString }],
            isBusy: NullableBool.TRUE,
          });
          const response = await swarmingClient.ListBots(request);
          return response.items;
        },
        staleTime: 30000,
        refetchInterval: 60000,
        Client: queryClient,
      };
    }),
  });

  // 3. Aggregate the results from all queries into a single map and state.
  const map = new Map<string, string>();
  // Find the first error object among the queries.
  const error = queries.find((q) => q.error)?.error || null;
  // The overall state is an error if any of the queries have an error.
  const isError = queries.some((q) => q.isError);
  // The overall state is pending if any of the queries are pending.
  const isPending = queries.some((q) => q.isPending);

  // Combine data from all successful queries.
  if (!isPending && !isError) {
    for (const query of queries) {
      if (query.data) {
        for (const bot of query.data) {
          if (bot.taskId) {
            const dutIdDimension = bot.dimensions?.find(
              (dim) => dim.key === 'dut_id',
            );
            if (dutIdDimension?.value?.length) {
              map.set(dutIdDimension.value[0], bot.taskId);
            }
          }
        }
      }
    }
  }
  return { map, error, isError, isPending };
};
