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

import { useQueryClient } from '@tanstack/react-query';

import { useDevices, useListDevicesQueryKey } from '@/fleet/hooks/use_devices';
import {
  Device,
  ListDevicesRequest,
  ListDevicesResponse,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

/**
 * Queries for a device using ListDevices query with a single id filter
 * and serves cached data from previous ListDevices queries while loading
 *
 * @param deviceId - the id of the device to query
 * @returns the `Device` or `undefined` if the query is loading and no device is present in the cache
 */
export const useDeviceData = (deviceId: string): Device | undefined => {
  const queryClient = useQueryClient();
  const listDevicesQueryKey = useListDevicesQueryKey();

  const queriesData =
    queryClient.getQueriesData<ListDevicesResponse>(listDevicesQueryKey);

  const request = ListDevicesRequest.fromPartial({
    pageSize: 1,
    pageToken: undefined,
    filter: 'id = ' + deviceId,
  });
  const devicesQuery = useDevices(request);

  if (devicesQuery.data) return devicesQuery.data.devices[0];

  for (const query of queriesData) {
    if (!query[1]) continue;
    for (const device of query[1].devices) {
      if (device.id === deviceId) {
        return device;
      }
    }
  }

  return undefined;
};
