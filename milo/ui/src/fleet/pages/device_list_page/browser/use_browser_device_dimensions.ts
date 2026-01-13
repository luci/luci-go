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

import { QueryKey, useQuery } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { PERSIST_INDEXED_DB } from '@/fleet/constants/caching_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { GetBrowserDeviceDimensionsRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const useBrowserDeviceDimensions = () => {
  const { identity } = useAuthState();
  const client = useFleetConsoleClient();

  const queryKey: QueryKey = [
    'fleet-console',
    identity,
    'browserDeviceDimensions',
    PERSIST_INDEXED_DB,
  ];

  const devicesQuery = useQuery({
    queryKey: queryKey,
    queryFn: client.GetBrowserDeviceDimensions.query(
      GetBrowserDeviceDimensionsRequest.create(),
    ).queryFn,
    // GetDeviceDimensions updates infrequently and is somewhat
    // expensive to call, so it's okay to refresh less often.
    staleTime: 1000 * 60 * 5, // 5 minutes
    gcTime: Infinity,
  });

  return devicesQuery;
};
