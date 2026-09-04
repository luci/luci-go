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

import {
  QueryKey,
  useQuery,
  UndefinedInitialDataOptions,
} from '@tanstack/react-query';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ListAndroidDevicesRequest,
  ListAndroidDevicesResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AndroidPageWorkspace } from '../workspaces';

export const useListAndroidDevicesQueryKey = (
  request?: ListAndroidDevicesRequest,
  workspace?: AndroidPageWorkspace,
) => {
  const { identity } = useAuthState();

  let queryKey: QueryKey = ['fleet-console', identity, 'listDevices'];
  if (workspace) {
    queryKey = [...queryKey, workspace];
  }
  if (request) {
    queryKey = [...queryKey, request];
  }
  return queryKey;
};

export const useAndroidDevices = (
  request: ListAndroidDevicesRequest,
  workspace: AndroidPageWorkspace,
  options?: Partial<
    UndefinedInitialDataOptions<
      ListAndroidDevicesResponse,
      Error,
      ListAndroidDevicesResponse,
      readonly unknown[]
    >
  >,
) => {
  const client = useFleetConsoleClient();
  const queryKey = useListAndroidDevicesQueryKey(request, workspace);
  const devicesQuery = useQuery({
    queryKey: queryKey,
    queryFn: client.ListAndroidDevices.query(request).queryFn,
    placeholderData: (previousData, previousQuery) => {
      // If transitioning between workspaces (e.g. Android <-> Pixel), do not
      // retain previous devices as placeholders to avoid confusing the user.
      const previousWorkspace = previousQuery?.queryKey.find(
        (key): key is AndroidPageWorkspace =>
          key === 'Android' || key === 'Pixel',
      );
      if (workspace && previousWorkspace && previousWorkspace !== workspace) {
        return undefined;
      }
      return previousData;
    },
    ...options,
  });
  return devicesQuery;
};
