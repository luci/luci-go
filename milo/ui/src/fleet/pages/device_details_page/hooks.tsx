// Copyright 2025 The LUCI Authors.
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

import { useQuery } from '@tanstack/react-query';

import { DecoratedClient } from '@/common/hooks/prpc_query';
import {
  BotsClientImpl,
  BotsRequest,
  BotTasksRequest,
  SortQuery,
  StateQuery,
  TaskResultResponse,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

export const useBotId = (
  client: DecoratedClient<BotsClientImpl>,
  dutId: string,
): {
  botId: string;
  botFound: boolean;
  error: unknown;
  isError: boolean;
  isLoading: boolean;
} => {
  // Fetch the first bot associated to the given `dut_id`.
  const { data, error, isError, isLoading } = useQuery({
    ...client.ListBots.query(
      BotsRequest.fromPartial({
        limit: 1,
        dimensions: [{ key: 'dut_id', value: dutId }],
      }),
    ),
    refetchInterval: 60000,
  });

  let botId = '';
  let botFound = false;
  if (!isError && !isLoading && data.items.length) {
    botFound = true;
    botId = data.items[0].botId;
  }

  return { botId, botFound, error, isError, isLoading };
};

export const useTasks = (
  client: DecoratedClient<BotsClientImpl>,
  botId: string,
): {
  tasks: readonly TaskResultResponse[] | undefined;
  error: unknown;
  isError: boolean;
  isLoading: boolean;
} => {
  // Fetch tasks for the associated bot.
  const { data, error, isError, isLoading } = useQuery({
    ...client.ListBotTasks.query(
      BotTasksRequest.fromPartial({
        botId: botId,
        limit: 30,
        state: StateQuery.QUERY_ALL,
        sort: SortQuery.QUERY_STARTED_TS,
      }),
    ),
    refetchInterval: 60000,
    // The query will not execute until `botId` is available.
    enabled: botId !== '',
  });

  return { tasks: data?.items, error, isError, isLoading };
};
