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

import { useQueryClient } from '@tanstack/react-query';

import {
  useAndroidDevices,
  useListAndroidDevicesQueryKey,
} from '@/fleet/hooks/use_android_devices';
import { escapeAipValue } from '@/fleet/utils/search_param';
import {
  AndroidDevice,
  ListAndroidDevicesRequest,
  ListAndroidDevicesResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export type UseAndroidDeviceDataResult = {
  error?: unknown;
  isError: boolean;
  isLoading: boolean;
  device?: AndroidDevice;
};

export const useAndroidDeviceData = (
  deviceId: string,
): UseAndroidDeviceDataResult => {
  const queryClient = useQueryClient();
  const listDevicesQueryKey = useListAndroidDevicesQueryKey();

  const queriesData = queryClient.getQueriesData<ListAndroidDevicesResponse>({
    queryKey: listDevicesQueryKey,
  });

  const request = ListAndroidDevicesRequest.fromPartial({
    pageSize: 1,
    filter: `id = "${escapeAipValue(deviceId)}"`,
  });
  const { data, error, isError, isLoading } = useAndroidDevices(request);

  if (data) {
    return {
      error,
      isError,
      isLoading,
      device: data.devices?.[0] ?? undefined,
    };
  }

  if (isLoading) {
    for (const query of queriesData) {
      const cachedDevice = query[1]?.devices?.find((d) => d.id === deviceId);
      if (cachedDevice) {
        return {
          isError: false,
          isLoading: false,
          device: cachedDevice,
        };
      }
    }
  }

  return {
    error,
    isError,
    isLoading,
  };
};
