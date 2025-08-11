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
  BotInfo,
  BotsClientImpl,
  BotsRequest,
  BotTasksRequest,
  SortQuery,
  StateQuery,
  TaskResultResponse,
  TasksClientImpl,
  TasksWithPerfRequest,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

export const useBot = (
  client: DecoratedClient<BotsClientImpl>,
  dutId: string,
): {
  info: BotInfo | undefined;
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

  let info = undefined;
  let botFound = false;
  if (!isError && !isLoading && data && data.items.length) {
    info = data.items[0];
    botFound = true;
  }

  return { info, botFound, error, isError, isLoading };
};

export const useBotTasks = ({
  client,
  botId,
  limit,
  pageToken,
}: {
  client: DecoratedClient<BotsClientImpl>;
  botId: string;
  limit: number;
  pageToken: string;
}): {
  tasks: readonly TaskResultResponse[] | undefined;
  nextPageToken: string;
  error: unknown;
  isError: boolean;
  isLoading: boolean;
} => {
  // Fetch tasks for the associated bot.
  const { data, error, isError, isLoading } = useQuery({
    ...client.ListBotTasks.query(
      BotTasksRequest.fromPartial({
        botId: botId,
        limit: limit,
        cursor: pageToken,
        state: StateQuery.QUERY_ALL,
        sort: SortQuery.QUERY_STARTED_TS,
      }),
    ),
    refetchInterval: 60000,
    // The query will not execute until `botId` is available.
    enabled: botId !== '',
  });

  return {
    tasks: data?.items,
    nextPageToken: data?.cursor || '',
    error,
    isError,
    isLoading,
  };
};

// TODO: b/436654106 look into cleaning up swarming hooks
export const useTasks = ({
  client,
  tags,
  limit,
  pageToken,
  startTime,
}: {
  client: DecoratedClient<TasksClientImpl>;
  tags: string[];
  limit: number;
  pageToken?: string;
  startTime?: string;
}): {
  tasks: readonly TaskResultResponse[] | undefined;
  nextPageToken: string;
  error: unknown;
  isError: boolean;
  isLoading: boolean;
} => {
  const { data, error, isError, isLoading } = useQuery({
    ...client.ListTasks.query(
      TasksWithPerfRequest.fromPartial({
        tags: tags,
        limit: limit,
        cursor: pageToken,
        state: StateQuery.QUERY_ALL,
        sort: SortQuery.QUERY_CREATED_TS,
        start: startTime,
      }),
    ),
    refetchInterval: 60000,
  });

  return {
    tasks: data?.items,
    nextPageToken: data?.cursor || '',
    error,
    isError,
    isLoading,
  };
};
