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

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { useAndroidDevices } from '@/fleet/hooks/use_android_devices';
import {
  AndroidDevice,
  ListAndroidDevicesRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export type UseAndroidDeviceDataResult = {
  error?: unknown;
  isError: boolean;
  isLoading: boolean;
  device?: AndroidDevice;
};

export const useAndroidDeviceData = (
  deviceId: string,
): UseAndroidDeviceDataResult => {
  const request = ListAndroidDevicesRequest.fromPartial({
    pageSize: 1,
    filter: stringifyFilters({ id: [deviceId] }),
  });
  const { data, error, isError, isLoading } = useAndroidDevices(request);

  return {
    error,
    isError,
    isLoading,
    device: data?.devices?.[0] ?? undefined,
  };
};
