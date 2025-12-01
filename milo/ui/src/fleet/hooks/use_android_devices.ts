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

import { QueryKey, useQuery, keepPreviousData } from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { ListAndroidDevicesRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const useListAndroidDevicesQueryKey = (request?: ListAndroidDevicesRequest) => {
  const { identity } = useAuthState();

  let queryKey: QueryKey = ['fleet-console', identity, 'listDevices'];
  if (request) {
    queryKey = [...queryKey, request];
  }
  return queryKey;
};

export const useAndroidDevices = (request: ListAndroidDevicesRequest) => {
  const client = useFleetConsoleClient();
  const queryKey = useListAndroidDevicesQueryKey(request);

  const devicesQuery = useQuery({
    queryKey: queryKey,
    queryFn: client.ListAndroidDevices.query(request).queryFn,
    placeholderData: keepPreviousData, // avoid loading while switching page
    select: (data) => ({
      ...data,
      devices: data.devices.map((d) => {
        const omnilabRunTarget = d.omnilabSpec?.labels['run_target'];
        if (!omnilabRunTarget) return d;

        delete d.omnilabSpec?.labels['run_target'];
        d.omnilabSpec.labels['omnilab-run_target'] = omnilabRunTarget;
        return d;
      }),
    }),
  });

  return devicesQuery;
};
