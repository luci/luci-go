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

import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ListTaskHistoryRequest,
  TaskHistoryItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface UseTaskHistoryOptions {
  dutId: string;
  pageSize?: number;
  pageToken?: string;
  enabled?: boolean;
}

export interface UseTaskHistoryResult {
  tasks: readonly TaskHistoryItem[] | undefined;
  nextPageToken: string;
  error: unknown;
  isError: boolean;
  isLoading: boolean;
}

export const useTaskHistory = ({
  dutId,
  pageSize = 50,
  pageToken = '',
  enabled = true,
}: UseTaskHistoryOptions): UseTaskHistoryResult => {
  const client = useFleetConsoleClient();

  const { data, error, isError, isLoading } = useQuery({
    ...client.ListTaskHistory.query(
      ListTaskHistoryRequest.fromPartial({
        dutId,
        pageSize,
        pageToken,
      }),
    ),
    placeholderData: keepPreviousData,
    refetchInterval: 60000,
    enabled: enabled && !!dutId,
  });

  return {
    tasks: data?.tasks,
    nextPageToken: data?.nextPageToken ?? '',
    error,
    isError,
    isLoading,
  };
};
