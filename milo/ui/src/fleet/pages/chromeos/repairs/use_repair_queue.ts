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

import { useQuery } from '@tanstack/react-query';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { ListRepairQueueRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const REPAIR_QUEUE_QUERY_KEY = ['repairQueue'] as const;

export const useRepairQueue = (request: ListRepairQueueRequest) => {
  const client = useFleetConsoleClient();
  return useQuery({
    ...client.ListRepairQueue.query(request),
    queryKey: [...REPAIR_QUEUE_QUERY_KEY, request],
    refetchInterval: 10000,
  });
};
