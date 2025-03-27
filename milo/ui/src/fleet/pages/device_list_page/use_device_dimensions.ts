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

import { QueryKey, useQuery } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

export const useDeviceDimensions = () => {
  const { identity } = useAuthState();
  const client = useFleetConsoleClient();

  const queryKey: QueryKey = [
    'fleet-console',
    identity,
    'deviceDimensions',
    'persist-local-storage',
  ];

  const devicesQuery = useQuery({
    queryKey: queryKey,
    queryFn: client.GetDeviceDimensions.query({}).queryFn,
    // By default staleTime is zero,
    // so it updates the cache data every time.
    cacheTime: Infinity,
  });

  return devicesQuery;
};
