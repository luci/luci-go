// Copyright 2024 The LUCI Authors.
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

import { QueryKey, useQuery, keepPreviousData } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { ListDevicesRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const useListDevicesQueryKey = (request?: ListDevicesRequest) => {
  const { identity } = useAuthState();

  let queryKey: QueryKey = ['fleet-console', identity, 'listDevices'];
  if (request) {
    queryKey = [...queryKey, request];
  }
  return queryKey;
};

export const useDevices = (request: ListDevicesRequest) => {
  const client = useFleetConsoleClient();
  const queryKey = useListDevicesQueryKey(request);

  const devicesQuery = useQuery({
    queryKey: queryKey,
    queryFn: client.ListDevices.query(request).queryFn,
    placeholderData: keepPreviousData, // avoid loading while switching page
  });

  return devicesQuery;
};
